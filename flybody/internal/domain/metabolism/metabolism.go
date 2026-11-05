// Package metabolism belongs to the Domain Layer (Abdomen Segment).
// It models the fat body, gut energy reserves (glycogen/ATP), and internal homeostatic drives.
package metabolism

import (
	"fmt"
	"time"

	"github.com/jhinw/flybrain/flybody/internal/domain/organ"
)

// DriveState holds the 5 core internal homeostatic drives in [0.0, 1.0].
type DriveState struct {
	Hunger    float64 `json:"hunger"`
	Fear      float64 `json:"fear"`
	Fatigue   float64 `json:"fatigue"`
	Curiosity float64 `json:"curiosity"`
	Groom     float64 `json:"groom"`
}

// SensationContext summarizes acute sensory events needed to modulate internal drives.
type SensationContext struct {
	Touched        bool    `json:"touched"`
	WindStrength   float64 `json:"wind_strength"`
	DangerOdor     bool    `json:"danger_odor"`
	LoomingLevel   float64 `json:"looming_level"`
	LightLevel     float64 `json:"light_level"`
}

// MetabolicTissue belongs to the Domain Layer (Abdomen Segment).
// Primary Responsibility: Maintain hemolymph glycogen/ATP energy reserves, evolve the 5 internal
// drives (Hunger, Fear, Fatigue, Curiosity, Groom), emit DRIVE_* neural pulses, and settle
// metabolic energy expenditure and nutrient intake after effector actuation.
type MetabolicTissue struct {
	GlycogenReserve float64    `json:"glycogen_reserve"` // [0.0, 1.0] (1.0 = 100% full reserve)
	ATPLevel        float64    `json:"atp_level"`        // [0.0, 1.0]
	Drives          DriveState `json:"drives"`
	WasMoving       bool       `json:"was_moving"`
	WasFeeding      bool       `json:"was_feeding"`
	WasGrooming     bool       `json:"was_grooming"`
}

// NewMetabolicTissue constructs a MetabolicTissue initialized to the baseline drives from connectome.js.
func NewMetabolicTissue() *MetabolicTissue {
	return &MetabolicTissue{
		GlycogenReserve: 0.784,
		ATPLevel:        0.850,
		Drives: DriveState{
			Hunger:    0.30,
			Fear:      0.00,
			Fatigue:   0.00,
			Curiosity: 0.50,
			Groom:     0.10,
		},
	}
}

func (m *MetabolicTissue) ID() string                  { return "metabolic_tissue" }
func (m *MetabolicTissue) Segment() organ.SegmentType { return organ.SegmentAbdomen }

// CollectDriveSpikes updates the 5 internal drives based on physiological state and acute sensation,
// then emits proportional DRIVE_* afferent spikes matching BRAIN.update() in connectome.js.
func (m *MetabolicTissue) CollectDriveSpikes(ctx SensationContext, dt time.Duration) []organ.AfferentSpike {
	d := &m.Drives

	// 1. Hunger: increases over time, decreases strongly when feeding
	d.Hunger += 0.005
	if m.WasFeeding {
		d.Hunger -= 0.30
	}

	// 2. Fear: spikes on touch, strong wind, danger odor, or looming, then decays exponentially
	if ctx.Touched {
		d.Fear += 0.30
	}
	if ctx.WindStrength > 0.5 {
		d.Fear += 0.20 * ctx.WindStrength
	}
	if ctx.DangerOdor {
		d.Fear += 0.25
	}
	if ctx.LoomingLevel > 0.3 {
		d.Fear += 0.35 * ctx.LoomingLevel
	}
	d.Fear *= 0.85

	// 3. Fatigue: increases when moving (faster in darkness), decreases when resting
	if m.WasMoving {
		fatigueGain := 0.003
		if ctx.LightLevel < 0.3 {
			fatigueGain = 0.006
		}
		d.Fatigue += fatigueGain
	} else {
		d.Fatigue -= 0.01
	}

	// 4. Curiosity: mild homeostatic drift around 0.5 (dampened in darkness)
	if ctx.LightLevel < 0.3 {
		d.Curiosity = d.Curiosity*0.96 + 0.35*0.04
	} else {
		d.Curiosity = d.Curiosity*0.96 + 0.52*0.04
	}

	// 5. Grooming urge: accumulates over time, spikes on touch, drops when grooming
	d.Groom += 0.008
	if ctx.Touched {
		d.Groom += 0.20
	}
	if m.WasGrooming {
		d.Groom -= 0.50
	}

	// Clamp all drives to [0.0, 1.0]
	d.Hunger = clamp01(d.Hunger)
	d.Fear = clamp01(d.Fear)
	d.Fatigue = clamp01(d.Fatigue)
	d.Curiosity = clamp01(d.Curiosity)
	d.Groom = clamp01(d.Groom)

	// Emit proportional drive pulses into the brain (matching connectome.js lines 279-302)
	var spikes []organ.AfferentSpike
	if d.Hunger > 0.2 {
		pulses := 1
		if d.Hunger > 0.6 {
			pulses = 3
		} else if d.Hunger > 0.4 {
			pulses = 2
		}
		spikes = append(spikes, organ.AfferentSpike{
			SourceOrgan: m.ID(),
			TargetGroup: "DRIVE_HUNGER",
			Scale:       d.Hunger,
			Pulses:      pulses,
		})
	}
	if d.Fear > 0.05 {
		pulses := 1
		if d.Fear > 0.5 {
			pulses = 3
		} else if d.Fear > 0.2 {
			pulses = 2
		}
		spikes = append(spikes, organ.AfferentSpike{
			SourceOrgan: m.ID(),
			TargetGroup: "DRIVE_FEAR",
			Scale:       d.Fear,
			Pulses:      pulses,
		})
	}
	if d.Fatigue > 0.3 {
		spikes = append(spikes, organ.AfferentSpike{
			SourceOrgan: m.ID(),
			TargetGroup: "DRIVE_FATIGUE",
			Scale:       d.Fatigue,
			Pulses:      1,
		})
	}
	if d.Curiosity > 0.2 {
		pulses := 1
		if d.Curiosity > 0.5 {
			pulses = 2
		}
		spikes = append(spikes, organ.AfferentSpike{
			SourceOrgan: m.ID(),
			TargetGroup: "DRIVE_CURIOSITY",
			Scale:       d.Curiosity,
			Pulses:      pulses,
		})
	}
	if d.Groom > 0.3 {
		spikes = append(spikes, organ.AfferentSpike{
			SourceOrgan: m.ID(),
			TargetGroup: "DRIVE_GROOM",
			Scale:       d.Groom,
			Pulses:      1,
		})
	}

	return spikes
}

// Settle updates glycogen and ATP reserves from effector work and feeding intake.
func (m *MetabolicTissue) Settle(kin organ.KinematicDelta, dt time.Duration) {
	m.WasMoving = kin.IsMoving
	m.WasFeeding = kin.IsFeeding
	m.WasGrooming = kin.IsGrooming

	m.GlycogenReserve = clamp01(m.GlycogenReserve - kin.EnergyExpended + kin.SugarIngested)
	if kin.IsMoving {
		m.ATPLevel = clamp01(m.ATPLevel - kin.EnergyExpended*1.5)
	} else {
		// Oxidative phosphorylation recovers ATP from glycogen while resting
		m.ATPLevel = clamp01(m.ATPLevel + 0.008*m.GlycogenReserve)
	}
}

func (m *MetabolicTissue) Summary() string {
	return fmt.Sprintf(
		"Glycogen=%.1f%% ATP=%.1f%% | Hunger=%.2f Fear=%.2f Fatigue=%.2f Curiosity=%.2f Groom=%.2f",
		m.GlycogenReserve*100, m.ATPLevel*100,
		m.Drives.Hunger, m.Drives.Fear, m.Drives.Fatigue, m.Drives.Curiosity, m.Drives.Groom,
	)
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
