// Package body belongs to the Domain Layer.
// It defines the VirtualBody aggregate root, organizing the three anatomical segments
// (Head, Thorax, Abdomen), routing stimuli to organs, collecting afferent spikes,
// distributing efferent motor commands, and integrating local 3D relative kinematics.
package body

import (
	"math"
	"time"

	"github.com/jhinw/flybrain/flybody/internal/domain/metabolism"
	"github.com/jhinw/flybrain/flybody/internal/domain/organ"
)

// BehaviorMode describes the emergent whole-body behavioral state of the fruit fly.
type BehaviorMode string

const (
	BehaviorIdle         BehaviorMode = "IDLE_STAND"
	BehaviorWalkExplore  BehaviorMode = "WALK_EXPLORE"
	BehaviorWalkApproach BehaviorMode = "WALK_APPROACH"
	BehaviorTurnOrient   BehaviorMode = "TURN_ORIENT"
	BehaviorFeed         BehaviorMode = "FEED_PROBOSCIS"
	BehaviorGroom        BehaviorMode = "GROOM_CLEAN"
	BehaviorEscapeJump   BehaviorMode = "ESCAPE_JUMP"
	BehaviorFlight       BehaviorMode = "FLIGHT_CRUISE"
)

// Pose3D tracks the local origin-free 3D displacement and orientation of the fly body.
// In Stage 2 (Decentralized 3D World), this local pose integrates with 3D Vivaldi spring relaxation.
type Pose3D struct {
	DispXMM    float64 `json:"disp_x_mm"`
	DispYMM    float64 `json:"disp_y_mm"`
	DispZMM    float64 `json:"disp_z_mm"`
	HeadingDeg float64 `json:"heading_deg"` // Horizontal compass azimuth [0..360)
	PitchDeg   float64 `json:"pitch_deg"`
	RollDeg    float64 `json:"roll_deg"`
}

// VirtualBody belongs to the Domain Layer.
// Primary Responsibility: Aggregate all sensory organs, motor effectors, and metabolic tissue
// across the Head, Thorax, and Abdomen segments, acting as the single subject responsible for
// whole-body sensation, actuation, and physiological state reporting.
type VirtualBody struct {
	NodeID       string                      `json:"node_id"`
	Pose         Pose3D                      `json:"pose"`
	Behavior     BehaviorMode                `json:"behavior"`
	Eyes         *organ.CompoundEye          `json:"eyes"`
	Antennae     *organ.Antenna              `json:"antennae"`
	Proboscis    *organ.Proboscis            `json:"proboscis"`
	Neck         *organ.NeckJoint            `json:"neck"`
	Legs         *organ.HexapodLegs          `json:"legs"`
	Wings        *organ.WingPair             `json:"wings"`
	Bristles     *organ.BristleArray         `json:"bristles"`
	Metabolism   *metabolism.MetabolicTissue `json:"metabolism"`
	LastKinDelta organ.KinematicDelta        `json:"last_kinematic_delta"`
}

// NewVirtualBody assembles a complete adult Drosophila virtual body with all organs attached.
func NewVirtualBody(nodeID string) *VirtualBody {
	if nodeID == "" {
		nodeID = "peer-fly-01"
	}
	return &VirtualBody{
		NodeID:     nodeID,
		Behavior:   BehaviorIdle,
		Eyes:       organ.NewCompoundEye(),
		Antennae:   organ.NewAntenna(),
		Proboscis:  organ.NewProboscis(),
		Neck:       organ.NewNeckJoint(),
		Legs:       organ.NewHexapodLegs(),
		Wings:      organ.NewWingPair(),
		Bristles:   organ.NewBristleArray(),
		Metabolism: metabolism.NewMetabolicTissue(),
	}
}

// CollectAfferentSpikes asks every sensory receptor organ across Head, Thorax, and Abdomen
// to adapt and report its afferent neural spikes for the current tick.
func (b *VirtualBody) CollectAfferentSpikes(dt time.Duration) []organ.AfferentSpike {
	b.Eyes.WasMovingLastTic = b.LastKinDelta.IsMoving
	receptors := []organ.SensoryReceptor{
		b.Eyes,
		b.Antennae,
		b.Proboscis,
		b.Bristles,
		b.Legs,
	}
	var all []organ.AfferentSpike
	for _, r := range receptors {
		all = append(all, r.CollectAfferentSpikes(dt)...)
	}
	return all
}

// SensationSummary builds the acute sensory context required by the metabolic drive system.
func (b *VirtualBody) SensationSummary() metabolism.SensationContext {
	maxLoom := math.Max(b.Eyes.Left.LoomingLevel, b.Eyes.Right.LoomingLevel)
	dangerMean := 0.5 * (b.Antennae.DangerOdorLeft + b.Antennae.DangerOdorRight)
	return metabolism.SensationContext{
		Touched:      b.Bristles.Deflection > 0.15,
		WindStrength: b.Antennae.JOVibration,
		DangerOdor:   dangerMean > 0.15,
		LoomingLevel: maxLoom,
		LightLevel:   b.Eyes.AmbientLight,
	}
}

// CombinedLateralBias merges stereo olfactory gradient bias and bilateral visual bias.
func (b *VirtualBody) CombinedLateralBias() float64 {
	bias := b.Antennae.StereoOdorBias + b.Eyes.VisualTurnBias
	if bias < -1.0 {
		return -1.0
	}
	if bias > 1.0 {
		return 1.0
	}
	return bias
}

// ActuateEffectors distributes the brain's motor readout to all motor effectors
// (NeckJoint, Proboscis, HexapodLegs, WingPair) and synthesizes the whole-body kinematic delta.
func (b *VirtualBody) ActuateEffectors(readout organ.MotorReadout, dt time.Duration) organ.KinematicDelta {
	readout.LateralBias = b.CombinedLateralBias()

	neckDelta := b.Neck.ApplyEfferentDrive(readout, dt)
	probDelta := b.Proboscis.ApplyEfferentDrive(readout, dt)
	wingDelta := b.Wings.ApplyEfferentDrive(readout, dt)
	legDelta := b.Legs.ApplyEfferentDrive(readout, dt)

	total := organ.KinematicDelta{
		ForwardSpeedMMPerSec: legDelta.ForwardSpeedMMPerSec,
		YawRateDegPerSec:     legDelta.YawRateDegPerSec,
		ClimbRateMMPerSec:    legDelta.ClimbRateMMPerSec + wingDelta.ClimbRateMMPerSec,
		IsMoving:             legDelta.IsMoving || wingDelta.IsMoving,
		IsFlying:             wingDelta.IsFlying,
		IsFeeding:            probDelta.IsFeeding,
		IsGrooming:           legDelta.IsGrooming,
		IsStartled:           legDelta.IsStartled || wingDelta.IsStartled,
		SugarIngested:        probDelta.SugarIngested,
		EnergyExpended:       neckDelta.EnergyExpended + probDelta.EnergyExpended + wingDelta.EnergyExpended + legDelta.EnergyExpended,
	}
	if wingDelta.IsFlying {
		total.ForwardSpeedMMPerSec = wingDelta.ForwardSpeedMMPerSec
		total.YawRateDegPerSec = wingDelta.YawRateDegPerSec
	}

	b.LastKinDelta = total
	b.Behavior = b.inferBehavior(total)
	return total
}

// SettleTick updates metabolic reserves and integrates local 3D relative displacement.
func (b *VirtualBody) SettleTick(kin organ.KinematicDelta, dt time.Duration) {
	b.Metabolism.Settle(kin, dt)

	sec := dt.Seconds()
	if sec <= 0 {
		sec = 0.05 // Default 50ms physiological tick (20 Hz)
	}
	b.Pose.HeadingDeg = math.Mod(b.Pose.HeadingDeg+kin.YawRateDegPerSec*sec+360.0, 360.0)
	headingRad := b.Pose.HeadingDeg * math.Pi / 180.0
	b.Pose.DispXMM += kin.ForwardSpeedMMPerSec * math.Cos(headingRad) * sec
	b.Pose.DispYMM += kin.ForwardSpeedMMPerSec * math.Sin(headingRad) * sec
	b.Pose.DispZMM = math.Max(0, b.Pose.DispZMM+kin.ClimbRateMMPerSec*sec)
	if !kin.IsFlying && b.Pose.DispZMM > 0 {
		// Gravity settles grounded fly back to z = 0
		b.Pose.DispZMM = math.Max(0, b.Pose.DispZMM-4.0*sec)
	}
}

func (b *VirtualBody) inferBehavior(kin organ.KinematicDelta) BehaviorMode {
	switch {
	case kin.IsStartled && kin.IsFlying:
		return BehaviorEscapeJump
	case kin.IsFlying:
		return BehaviorFlight
	case kin.IsStartled:
		return BehaviorEscapeJump
	case kin.IsFeeding:
		return BehaviorFeed
	case kin.IsGrooming:
		return BehaviorGroom
	case kin.IsMoving:
		foodSignal := 0.5 * (b.Antennae.FoodOdorLeft + b.Antennae.FoodOdorRight)
		uvColor := 0.5 * (b.Eyes.Left.R7R8Chromatic + b.Eyes.Right.R7R8Chromatic)
		if foodSignal > 0.15 || uvColor > 0.65 {
			return BehaviorWalkApproach
		}
		if math.Abs(kin.YawRateDegPerSec) > 30.0 {
			return BehaviorTurnOrient
		}
		return BehaviorWalkExplore
	default:
		return BehaviorIdle
	}
}
