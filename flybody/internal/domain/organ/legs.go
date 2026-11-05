package organ

import (
	"fmt"
	"math"
	"time"
)

// TripodState represents the stance/swing state of one alternating tripod group (A: L1,R2,L3 or B: R1,L2,R3).
type TripodState struct {
	PhaseLabel   string  `json:"phase_label"`   // "STANCE", "SWING", "GROOM", or "REST"
	PushForce    float64 `json:"push_force"`    // [0.0, 1.0]
	CoxaAngleDeg float64 `json:"coxa_angle_deg"` // Protraction (+) / retraction (-) angle
	LiftHeightMM float64 `json:"lift_height_mm"`
}

// HexapodLegs belongs to the Domain Layer (Thorax Segment).
// Primary Responsibility: Actuate the six thoracic legs (L1..R3) in alternating tripod gait,
// front-leg grooming, or escape jumping based on MN_LEG_L1..R3, while providing chordotonal
// proprioceptive feedback (MECH_CHORD) to the brain.
type HexapodLegs struct {
	TripodPhaseDeg     float64     `json:"tripod_phase_deg"` // [0..360)
	WalkingSpeedMM     float64     `json:"walking_speed_mm_s"`
	YawRateDeg         float64     `json:"yaw_rate_deg_s"`
	GroomingActive     bool        `json:"grooming_active"`
	GroomBoutRemaining int         `json:"groom_bout_remaining"`
	JumpingActive      bool        `json:"jumping_active"`
	TripodA            TripodState `json:"tripod_a"` // L1, R2, L3
	TripodB            TripodState `json:"tripod_b"` // R1, L2, R3
}

// NewHexapodLegs constructs six thoracic legs in a standing rest posture.
func NewHexapodLegs() *HexapodLegs {
	return &HexapodLegs{
		TripodA: TripodState{PhaseLabel: "REST"},
		TripodB: TripodState{PhaseLabel: "REST"},
	}
}

func (l *HexapodLegs) ID() string           { return "hexapod_legs" }
func (l *HexapodLegs) Segment() SegmentType { return SegmentThorax }

// CollectAfferentSpikes emits femoral chordotonal organ proprioceptive feedback (MECH_CHORD) when moving.
func (l *HexapodLegs) CollectAfferentSpikes(dt time.Duration) []AfferentSpike {
	if l.WalkingSpeedMM > 0.15 {
		return []AfferentSpike{
			{
				SourceOrgan: l.ID(),
				TargetGroup: "MECH_CHORD",
				Scale:       1.0,
				Pulses:      1,
			},
		}
	}
	return nil
}

// ApplyEfferentDrive converts MN_LEG_L1..R3 and grooming/startle signals into tripod gait kinematics.
func (l *HexapodLegs) ApplyEfferentDrive(drive MotorReadout, dt time.Duration) KinematicDelta {
	walkLeft := drive.WalkLeftTotal()
	walkRight := drive.WalkRightTotal()
	walkTotal := walkLeft + walkRight
	groomScore := drive.GroomTotal()

	l.GroomingActive = false
	l.JumpingActive = false

	// Check escape jump (matching BEHAVIOR_THRESHOLDS.startle = 30 in fly-logic.js)
	if drive.Startle > 30 && (drive.LegL3+drive.LegR3) >= 6 {
		l.JumpingActive = true
		l.WalkingSpeedMM = 18.0
		l.YawRateDeg = drive.LateralBias * 95.0
		l.TripodA = TripodState{PhaseLabel: "JUMP_EXTEND", PushForce: 0.95, CoxaAngleDeg: -32.0, LiftHeightMM: 1.20}
		l.TripodB = TripodState{PhaseLabel: "JUMP_EXTEND", PushForce: 0.95, CoxaAngleDeg: -32.0, LiftHeightMM: 1.20}
		return KinematicDelta{
			ForwardSpeedMMPerSec: l.WalkingSpeedMM,
			YawRateDegPerSec:     l.YawRateDeg,
			ClimbRateMMPerSec:    8.5,
			IsMoving:             true,
			IsStartled:           true,
			EnergyExpended:       0.012,
		}
	}

	// Check front-leg / abdominal grooming dominance (matching BEHAVIOR_THRESHOLDS.groom = 8 in fly-logic.js
	// and BEHAVIOR_MIN_DURATION bout persistence across single-tick neural refractory periods)
	isPureFlightDominant := drive.FlightTotal() > 15 && drive.FlightTotal() > groomScore
	if groomScore > 8 && !isPureFlightDominant && drive.Proboscis <= 8 {
		l.GroomBoutRemaining = 2
	}
	if l.GroomBoutRemaining > 0 && !isPureFlightDominant && drive.Proboscis <= 8 {
		l.GroomBoutRemaining--
		l.GroomingActive = true
		l.WalkingSpeedMM = 0
		l.YawRateDeg = 0
		l.TripodPhaseDeg = math.Mod(l.TripodPhaseDeg+45.0, 360.0)
		l.TripodA = TripodState{PhaseLabel: "GROOM_RUB", PushForce: 0.35, CoxaAngleDeg: 18.0, LiftHeightMM: 0.45}
		l.TripodB = TripodState{PhaseLabel: "STANCE_ANCHOR", PushForce: 0.50, CoxaAngleDeg: 0.0, LiftHeightMM: 0.0}
		return KinematicDelta{
			IsGrooming:     true,
			EnergyExpended: 0.004,
		}
	}

	// Check walking locomotion (matching BEHAVIOR_THRESHOLDS.walk = 5 in fly-logic.js)
	if walkTotal > 5 && drive.FlightTotal() <= 15 && drive.Proboscis <= 8 {
		// Advance tripod gait phase proportional to total leg motor output
		stepAdvance := 35.0 + walkTotal*2.2
		l.TripodPhaseDeg = math.Mod(l.TripodPhaseDeg+stepAdvance, 360.0)
		l.WalkingSpeedMM = clampRange(walkTotal*0.14, 0.2, 12.0)

		// Asymmetric leg drive + stereo sensory bias produces turning yaw rate
		motorAsym := (walkLeft - walkRight) * 2.5
		stereoTurn := drive.LateralBias * 38.0
		l.YawRateDeg = clampRange(motorAsym+stereoTurn, -120.0, 120.0)

		rad := l.TripodPhaseDeg * math.Pi / 180.0
		sinPhase := math.Sin(rad)
		forceNorm := clamp01(walkTotal / 36.0)
		if sinPhase >= 0 {
			l.TripodA = TripodState{
				PhaseLabel:   "STANCE",
				PushForce:    0.4 + 0.5*forceNorm,
				CoxaAngleDeg: 22.0 * sinPhase,
				LiftHeightMM: 0.0,
			}
			l.TripodB = TripodState{
				PhaseLabel:   "SWING",
				PushForce:    0.05,
				CoxaAngleDeg: -18.0 * sinPhase,
				LiftHeightMM: 0.35 * sinPhase,
			}
		} else {
			l.TripodA = TripodState{
				PhaseLabel:   "SWING",
				PushForce:    0.05,
				CoxaAngleDeg: 18.0 * sinPhase,
				LiftHeightMM: -0.35 * sinPhase,
			}
			l.TripodB = TripodState{
				PhaseLabel:   "STANCE",
				PushForce:    0.4 + 0.5*forceNorm,
				CoxaAngleDeg: -22.0 * sinPhase,
				LiftHeightMM: 0.0,
			}
		}

		return KinematicDelta{
			ForwardSpeedMMPerSec: l.WalkingSpeedMM,
			YawRateDegPerSec:     l.YawRateDeg,
			IsMoving:             true,
			EnergyExpended:       0.003 + 0.004*forceNorm,
		}
	}

	// Standing rest posture
	l.WalkingSpeedMM = 0
	l.YawRateDeg = 0
	l.TripodA = TripodState{PhaseLabel: "REST", PushForce: 0.15, CoxaAngleDeg: 0, LiftHeightMM: 0}
	l.TripodB = TripodState{PhaseLabel: "REST", PushForce: 0.15, CoxaAngleDeg: 0, LiftHeightMM: 0}
	return KinematicDelta{
		EnergyExpended: 0.0005,
	}
}

func (l *HexapodLegs) Summary() string {
	return fmt.Sprintf(
		"Speed=%.2f mm/s YawRate=%+.1f°/s Phase=%.0f° | Tripod.A(L1,R2,L3)=%s(F=%.2f,Coxa=%+.0f°) | Tripod.B(R1,L2,R3)=%s(Lift=%.2fmm)",
		l.WalkingSpeedMM, l.YawRateDeg, l.TripodPhaseDeg,
		l.TripodA.PhaseLabel, l.TripodA.PushForce, l.TripodA.CoxaAngleDeg,
		l.TripodB.PhaseLabel, l.TripodB.LiftHeightMM,
	)
}
