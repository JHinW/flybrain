package organ

import (
	"fmt"
	"time"
)

// TouchStimulus represents a mechanical touch or noxious poke applied to a body segment.
type TouchStimulus struct {
	TargetLocation string  `json:"target_location"` // "head", "thorax", "abdomen", "leg_r1", etc.
	Force          float64 `json:"force"`           // [0.0, 1.0]; >= 0.85 also triggers nociception (NOCI)
	DurationTicks  int     `json:"duration_ticks"`
}

// BristleArray belongs to the Domain Layer (Distributed across Head/Thorax/Abdomen, anchored on Abdomen).
// Primary Responsibility: Transduce cuticular bristle deflection and noxious mechanical force
// into MECH_BRISTLE and NOCI afferent spikes.
type BristleArray struct {
	LastLocation   string         `json:"last_location"`
	Deflection     float64        `json:"deflection"`
	NociceptiveHit bool           `json:"nociceptive_hit"`
	ActiveStimulus *TouchStimulus `json:"active_stimulus,omitempty"`
	RemainingTicks int            `json:"remaining_ticks"`
}

// NewBristleArray constructs a calm cuticular bristle receptor array.
func NewBristleArray() *BristleArray {
	return &BristleArray{
		LastLocation: "none",
	}
}

func (b *BristleArray) ID() string           { return "cuticular_bristles" }
func (b *BristleArray) Segment() SegmentType { return SegmentAbdomen }

// PerceiveTouch assigns a mechanical touch task to the cuticular bristles.
func (b *BristleArray) PerceiveTouch(stim TouchStimulus) {
	stim.Force = clamp01(stim.Force)
	if stim.TargetLocation == "" {
		stim.TargetLocation = "thorax"
	}
	if stim.DurationTicks <= 0 {
		stim.DurationTicks = 1
	}
	cp := stim
	b.ActiveStimulus = &cp
	b.RemainingTicks = stim.DurationTicks
	if stim.Force >= 0.85 {
		b.NociceptiveHit = true
	}
}

// CollectAfferentSpikes emits MECH_BRISTLE and single-tick NOCI spikes.
func (b *BristleArray) CollectAfferentSpikes(dt time.Duration) []AfferentSpike {
	var spikes []AfferentSpike

	if b.ActiveStimulus != nil && b.RemainingTicks > 0 {
		s := b.ActiveStimulus
		b.LastLocation = s.TargetLocation
		b.Deflection = s.Force

		pulses := 1
		// Matching connectome.js lines 308-313: head/thorax touch delivers double dose (2 pulses)
		if s.TargetLocation == "head" || s.TargetLocation == "thorax" || s.TargetLocation == "leg_l1" || s.TargetLocation == "leg_r1" {
			pulses = 2
		}
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: b.ID(),
			TargetGroup: "MECH_BRISTLE",
			Scale:       s.Force / 0.65,
			Pulses:      pulses,
		})

		if b.NociceptiveHit {
			spikes = append(spikes, AfferentSpike{
				SourceOrgan: b.ID(),
				TargetGroup: "NOCI",
				Scale:       1.5,
				Pulses:      3,
			})
			b.NociceptiveHit = false // Single-tick burst
		}

		b.RemainingTicks--
		if b.RemainingTicks <= 0 {
			b.ActiveStimulus = nil
		}
	} else {
		b.Deflection *= 0.4
	}

	return spikes
}

func (b *BristleArray) Summary() string {
	return fmt.Sprintf("Bristles[LastTouch=%s Deflection=%.2f]", b.LastLocation, b.Deflection)
}

// NeckJoint belongs to the Domain Layer (Head Segment).
// Primary Responsibility: Actuate head yaw and pitch angles relative to the thorax in response to MN_HEAD.
type NeckJoint struct {
	PitchDeg float64 `json:"pitch_deg"` // Positive = head up, negative = head lowered to feed/groom
	YawDeg   float64 `json:"yaw_deg"`   // Positive = right turn, negative = left turn
}

// NewNeckJoint constructs a neutral neck joint.
func NewNeckJoint() *NeckJoint {
	return &NeckJoint{}
}

func (n *NeckJoint) ID() string           { return "neck_joint" }
func (n *NeckJoint) Segment() SegmentType { return SegmentHead }

// ApplyEfferentDrive updates head pitch and yaw angles based on MN_HEAD, feeding, and lateral steering bias.
func (n *NeckJoint) ApplyEfferentDrive(drive MotorReadout, dt time.Duration) KinematicDelta {
	if drive.Proboscis > 4 || drive.GroomTotal() > 8 {
		// Lower head when feeding or grooming
		n.PitchDeg = clampRange(n.PitchDeg*0.5-12.0*(drive.Head/10.0+0.4), -25.0, 25.0)
	} else {
		n.PitchDeg *= 0.7
	}
	targetYaw := drive.LateralBias * 20.0
	if drive.Head > 0 {
		targetYaw += (drive.WalkRightTotal() - drive.WalkLeftTotal()) * 0.4
	}
	n.YawDeg = clampRange(n.YawDeg*0.6+targetYaw*0.4, -35.0, 35.0)
	return KinematicDelta{
		EnergyExpended: (drive.Head / 20.0) * 0.001,
	}
}

func (n *NeckJoint) Summary() string {
	return fmt.Sprintf("Neck[Pitch=%+.1f° Yaw=%+.1f°]", n.PitchDeg, n.YawDeg)
}
