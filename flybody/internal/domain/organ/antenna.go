package organ

import (
	"fmt"
	"math"
	"time"
)

// OdorType categorizes chemical stimuli detected by antennal olfactory receptor neurons (ORNs).
type OdorType string

const (
	OdorFood   OdorType = "food"
	OdorDanger OdorType = "danger"
)

// AntennalStimulus describes chemical, airflow, or thermal stimuli arriving at the left and right antennae.
type AntennalStimulus struct {
	OdorType       OdorType `json:"odor_type"`
	LeftOdorConc   float64  `json:"left_odor_conc"`   // [0.0, 1.0]
	RightOdorConc  float64  `json:"right_odor_conc"`  // [0.0, 1.0]
	WindStrength   float64  `json:"wind_strength"`    // [0.0, 1.0]
	WindAzimuthDeg float64  `json:"wind_azimuth_deg"` // Wind source angle in local body frame
	Temperature    float64  `json:"temperature"`      // Normalized [0.0=cold, 0.5=preferred 25C, 1.0=hot]
	HasTemperature bool     `json:"has_temperature"`
	DurationTicks  int      `json:"duration_ticks"`
}

// Antenna belongs to the Domain Layer (Head Segment).
// Primary Responsibility: Transduce bilateral odor concentrations (stereo chemotaxis),
// Johnston's organ airflow/vibration (MECH_JO), and arista temperature (THERMO_WARM/COOL)
// into afferent spikes for the Antennal Lobe.
type Antenna struct {
	FoodOdorLeft     float64           `json:"food_odor_left"`
	FoodOdorRight    float64           `json:"food_odor_right"`
	DangerOdorLeft   float64           `json:"danger_odor_left"`
	DangerOdorRight  float64           `json:"danger_odor_right"`
	JOVibration      float64           `json:"jo_vibration"`
	WindAzimuthDeg   float64           `json:"wind_azimuth_deg"`
	Temperature      float64           `json:"temperature"` // [0..1], 0.5 is preferred 25°C
	StereoOdorBias   float64           `json:"stereo_odor_bias"`
	ActiveStimulus   *AntennalStimulus `json:"active_stimulus,omitempty"`
	RemainingTicks   int               `json:"remaining_ticks"`
}

// NewAntenna constructs a pair of antennae in baseline environmental conditions.
func NewAntenna() *Antenna {
	return &Antenna{
		Temperature: 0.50,
	}
}

func (a *Antenna) ID() string           { return "antennae_lr" }
func (a *Antenna) Segment() SegmentType { return SegmentHead }

// PerceiveAntennal assigns an olfactory, airflow, or thermal perception task to the antennae.
func (a *Antenna) PerceiveAntennal(stim AntennalStimulus) {
	stim.LeftOdorConc = clamp01(stim.LeftOdorConc)
	stim.RightOdorConc = clamp01(stim.RightOdorConc)
	stim.WindStrength = clamp01(stim.WindStrength)
	if stim.HasTemperature {
		stim.Temperature = clamp01(stim.Temperature)
	} else {
		stim.Temperature = a.Temperature
	}
	if stim.DurationTicks <= 0 {
		stim.DurationTicks = 1
	}
	cp := stim
	a.ActiveStimulus = &cp
	a.RemainingTicks = stim.DurationTicks
}

// CollectAfferentSpikes updates bilateral receptor concentrations and emits olfactory/mechano/thermo spikes.
func (a *Antenna) CollectAfferentSpikes(dt time.Duration) []AfferentSpike {
	var spikes []AfferentSpike

	if a.ActiveStimulus != nil && a.RemainingTicks > 0 {
		s := a.ActiveStimulus
		switch s.OdorType {
		case OdorDanger:
			a.DangerOdorLeft = s.LeftOdorConc
			a.DangerOdorRight = s.RightOdorConc
			a.FoodOdorLeft *= 0.5
			a.FoodOdorRight *= 0.5
		default:
			a.FoodOdorLeft = s.LeftOdorConc
			a.FoodOdorRight = s.RightOdorConc
			a.DangerOdorLeft *= 0.5
			a.DangerOdorRight *= 0.5
		}
		a.JOVibration = s.WindStrength
		a.WindAzimuthDeg = s.WindAzimuthDeg
		if s.HasTemperature {
			a.Temperature = s.Temperature
		}
		a.RemainingTicks--
		if a.RemainingTicks <= 0 {
			a.ActiveStimulus = nil
		}
	} else {
		// Natural receptor clearance / adaptation decay
		a.FoodOdorLeft *= 0.65
		a.FoodOdorRight *= 0.65
		a.DangerOdorLeft *= 0.65
		a.DangerOdorRight *= 0.65
		a.JOVibration *= 0.60
	}

	foodMean := 0.5 * (a.FoodOdorLeft + a.FoodOdorRight)
	dangerMean := 0.5 * (a.DangerOdorLeft + a.DangerOdorRight)

	// Stereo chemotaxis: turn toward higher food odor, or away from higher danger odor
	if dangerMean > 0.1 {
		a.StereoOdorBias = clampRange((a.DangerOdorLeft-a.DangerOdorRight)*1.5, -1.0, 1.0)
	} else {
		a.StereoOdorBias = clampRange((a.FoodOdorRight-a.FoodOdorLeft)*1.5, -1.0, 1.0)
	}

	if foodMean > 0.1 {
		pulses := 1
		if foodMean > 0.6 {
			pulses = 2
		}
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: a.ID(),
			TargetGroup: "OLF_ORN_FOOD",
			Scale:       math.Max(0.5, foodMean*1.2),
			Pulses:      pulses,
		})
	}
	if dangerMean > 0.1 {
		pulses := 1
		if dangerMean > 0.5 {
			pulses = 2
		}
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: a.ID(),
			TargetGroup: "OLF_ORN_DANGER",
			Scale:       math.Max(0.5, dangerMean*1.2),
			Pulses:      pulses,
		})
	}
	if a.JOVibration > 0.08 {
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: a.ID(),
			TargetGroup: "MECH_JO",
			Scale:       a.JOVibration,
			Pulses:      1,
		})
	}
	if a.Temperature > 0.65 {
		warmScale := (a.Temperature - 0.5) * 2.0
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: a.ID(),
			TargetGroup: "THERMO_WARM",
			Scale:       warmScale,
			Pulses:      1,
		})
	} else if a.Temperature < 0.35 {
		coolScale := (0.5 - a.Temperature) * 2.0
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: a.ID(),
			TargetGroup: "THERMO_COOL",
			Scale:       coolScale,
			Pulses:      1,
		})
	}

	return spikes
}

func (a *Antenna) Summary() string {
	biasDir := "Center"
	if a.StereoOdorBias > 0.05 {
		biasDir = "Right-Bias"
	} else if a.StereoOdorBias < -0.05 {
		biasDir = "Left-Bias"
	}
	return fmt.Sprintf(
		"FoodOdor=[L:%.2f R:%.2f Δ=%+.2f %s] | Danger=[L:%.2f R:%.2f] | JO_Wind=%.2f | Temp=%.2f",
		a.FoodOdorLeft, a.FoodOdorRight, a.FoodOdorRight-a.FoodOdorLeft, biasDir,
		a.DangerOdorLeft, a.DangerOdorRight, a.JOVibration, a.Temperature,
	)
}
