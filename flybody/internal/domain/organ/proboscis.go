package organ

import (
	"fmt"
	"time"
)

// TasteType categorizes gustatory chemicals contacting labellar or tarsal taste bristles.
type TasteType string

const (
	TasteSweet  TasteType = "sweet"
	TasteBitter TasteType = "bitter"
	TasteWater  TasteType = "water"
)

// GustatoryStimulus represents direct contact with a tastant (sugar, bitter toxin, or water).
type GustatoryStimulus struct {
	Taste         TasteType `json:"taste"`
	Concentration float64   `json:"concentration"` // [0.0, 1.0]
	DurationTicks int       `json:"duration_ticks"`
}

// Proboscis belongs to the Domain Layer (Head Segment).
// Primary Responsibility: Dual sensory receptor and motor effector for feeding.
// It transduces sweet/bitter/water contact into GUS_GRN_* spikes and extends in response to MN_PROBOSCIS.
type Proboscis struct {
	Extension      float64            `json:"extension"`       // [0.0 = retracted, 1.0 = fully extended]
	SweetLevel     float64            `json:"sweet_level"`     // [0.0, 1.0]
	BitterLevel    float64            `json:"bitter_level"`    // [0.0, 1.0]
	WaterLevel     float64            `json:"water_level"`     // [0.0, 1.0]
	IntakeRate     float64            `json:"intake_rate"`     // Sugar ingested during last tick
	ActiveStimulus *GustatoryStimulus `json:"active_stimulus,omitempty"`
	RemainingTicks int                `json:"remaining_ticks"`
}

// NewProboscis creates a retracted proboscis.
func NewProboscis() *Proboscis {
	return &Proboscis{}
}

func (p *Proboscis) ID() string           { return "proboscis" }
func (p *Proboscis) Segment() SegmentType { return SegmentHead }

// PerceiveTaste assigns a gustatory perception task to the proboscis taste bristles.
func (p *Proboscis) PerceiveTaste(stim GustatoryStimulus) {
	stim.Concentration = clamp01(stim.Concentration)
	if stim.DurationTicks <= 0 {
		stim.DurationTicks = 1
	}
	cp := stim
	p.ActiveStimulus = &cp
	p.RemainingTicks = stim.DurationTicks
}

// CollectAfferentSpikes emits gustatory receptor neuron spikes (GUS_GRN_SWEET, BITTER, WATER).
func (p *Proboscis) CollectAfferentSpikes(dt time.Duration) []AfferentSpike {
	var spikes []AfferentSpike

	if p.ActiveStimulus != nil && p.RemainingTicks > 0 {
		s := p.ActiveStimulus
		switch s.Taste {
		case TasteBitter:
			p.BitterLevel = s.Concentration
			p.SweetLevel *= 0.3
		case TasteWater:
			p.WaterLevel = s.Concentration
		default:
			p.SweetLevel = s.Concentration
			p.BitterLevel *= 0.3
		}
		p.RemainingTicks--
		if p.RemainingTicks <= 0 {
			p.ActiveStimulus = nil
		}
	} else {
		p.SweetLevel *= 0.5
		p.BitterLevel *= 0.5
		p.WaterLevel *= 0.5
	}

	if p.SweetLevel > 0.1 {
		pulses := 1
		if p.SweetLevel > 0.5 {
			pulses = 3 // Ensure GUS_GRN_SWEET crosses fireThreshold (22) to trigger SEZ_FEED -> MN_PROBOSCIS
		}
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: p.ID(),
			TargetGroup: "GUS_GRN_SWEET",
			Scale:       p.SweetLevel,
			Pulses:      pulses,
		})
	}
	if p.BitterLevel > 0.1 {
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: p.ID(),
			TargetGroup: "GUS_GRN_BITTER",
			Scale:       p.BitterLevel,
			Pulses:      3,
		})
	}
	if p.WaterLevel > 0.1 {
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: p.ID(),
			TargetGroup: "GUS_GRN_WATER",
			Scale:       p.WaterLevel,
			Pulses:      3,
		})
	}

	return spikes
}

// ApplyEfferentDrive actuates proboscis extension (PER) from MN_PROBOSCIS and calculates nutrient intake.
func (p *Proboscis) ApplyEfferentDrive(drive MotorReadout, dt time.Duration) KinematicDelta {
	targetExt := clamp01(drive.Proboscis / 14.0)
	if targetExt > 0.1 {
		p.Extension = clamp01(p.Extension*0.3 + targetExt*0.7)
	} else {
		p.Extension = clamp01(p.Extension * 0.4)
	}

	isFeeding := p.Extension > 0.35
	p.IntakeRate = 0
	if isFeeding {
		sugarAvail := p.SweetLevel
		if sugarAvail < 0.4 {
			sugarAvail = 0.6 // Active proboscis extension onto surface ingests available substrate
		}
		p.IntakeRate = p.Extension * sugarAvail * 0.25
	}

	return KinematicDelta{
		IsFeeding:      isFeeding,
		SugarIngested:  p.IntakeRate,
		EnergyExpended: p.Extension * 0.002,
	}
}

func (p *Proboscis) Summary() string {
	state := "RETRACTED"
	if p.Extension > 0.35 {
		state = "EXTENDED_FEEDING"
	}
	return fmt.Sprintf(
		"State=%s (Ext=%.2f) | Sweet=%.2f Bitter=%.2f | Intake=%.3f",
		state, p.Extension, p.SweetLevel, p.BitterLevel, p.IntakeRate,
	)
}
