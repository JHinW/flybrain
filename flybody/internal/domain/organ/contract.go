// Package organ belongs to the Domain Layer.
// It defines the RSI (Similarity) abstractions shared across all anatomical organs
// of the Drosophila virtual body, as well as concrete sensory and motor organ implementations.
package organ

import "time"

// SegmentType identifies which of the three anatomical tagmata owns an organ.
type SegmentType string

const (
	SegmentHead    SegmentType = "HEAD"
	SegmentThorax  SegmentType = "THORAX"
	SegmentAbdomen SegmentType = "ABDOMEN"
)

// AfferentSpike represents a sensory or internal drive signal sent from a body organ
// into a specific functional neuron group of the brain.
type AfferentSpike struct {
	SourceOrgan string  `json:"source_organ"`
	TargetGroup string  `json:"target_group"`
	Scale       float64 `json:"scale"`  // Multiplier for dendritic accumulation (1.0 = unscaled)
	Pulses      int     `json:"pulses"` // Number of spike pulses this tick (default 1)
}

// MotorReadout holds the efferent motor neuron activations drained from the brain
// at the end of a synaptic propagation tick, plus lateral sensory steering bias.
type MotorReadout struct {
	LegL1       float64 `json:"leg_l1"`
	LegR1       float64 `json:"leg_r1"`
	LegL2       float64 `json:"leg_l2"`
	LegR2       float64 `json:"leg_r2"`
	LegL3       float64 `json:"leg_l3"`
	LegR3       float64 `json:"leg_r3"`
	WingL       float64 `json:"wing_l"`
	WingR       float64 `json:"wing_r"`
	Proboscis   float64 `json:"proboscis"`
	Head        float64 `json:"head"`
	Abdomen     float64 `json:"abdomen"`
	Startle     float64 `json:"startle"`
	LateralBias float64 `json:"lateral_bias"` // Stereo sensory steering bias (-1.0 left .. +1.0 right)
}

// WalkLeftTotal returns total motor activation across the three left legs.
func (m MotorReadout) WalkLeftTotal() float64 {
	return m.LegL1 + m.LegL2 + m.LegL3
}

// WalkRightTotal returns total motor activation across the three right legs.
func (m MotorReadout) WalkRightTotal() float64 {
	return m.LegR1 + m.LegR2 + m.LegR3
}

// FlightTotal returns total motor activation across both wings.
func (m MotorReadout) FlightTotal() float64 {
	return m.WingL + m.WingR
}

// GroomTotal computes composite grooming activation from front legs, abdomen, and head.
func (m MotorReadout) GroomTotal() float64 {
	minFront := m.LegL1
	if m.LegR1 < minFront {
		minFront = m.LegR1
	}
	headContrib := 0.0
	if m.Abdomen > 0 {
		headContrib = m.Head
	}
	return m.Abdomen + headContrib + minFront
}

// KinematicDelta summarizes the physical work and motion produced by motor effectors during one tick.
type KinematicDelta struct {
	ForwardSpeedMMPerSec float64 `json:"forward_speed_mm_s"`
	YawRateDegPerSec     float64 `json:"yaw_rate_deg_s"`
	ClimbRateMMPerSec    float64 `json:"climb_rate_mm_s"`
	IsMoving             bool    `json:"is_moving"`
	IsFlying             bool    `json:"is_flying"`
	IsFeeding            bool    `json:"is_feeding"`
	IsGrooming           bool    `json:"is_grooming"`
	IsStartled           bool    `json:"is_startled"`
	SugarIngested        float64 `json:"sugar_ingested"`
	EnergyExpended       float64 `json:"energy_expended"`
}

// Organ is the foundational RSI interface shared by every physiological organ in the virtual body.
type Organ interface {
	ID() string
	Segment() SegmentType
	Summary() string
}

// SensoryReceptor is implemented by organs capable of transducing external stimuli
// into afferent neural spikes for the brain.
type SensoryReceptor interface {
	Organ
	CollectAfferentSpikes(dt time.Duration) []AfferentSpike
}

// MotorEffector is implemented by organs that actuate physical movement or posture changes
// in response to efferent motor neuron readouts from the brain.
type MotorEffector interface {
	Organ
	ApplyEfferentDrive(drive MotorReadout, dt time.Duration) KinematicDelta
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampRange(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
