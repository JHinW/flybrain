package organ

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// EyeSide identifies which compound eye receives a visual stimulus.
type EyeSide string

const (
	EyeSideLeft  EyeSide = "left"
	EyeSideRight EyeSide = "right"
	EyeSideBoth  EyeSide = "both"
)

// VisualStimulus describes an external optical input directed at the compound eyes.
type VisualStimulus struct {
	Side          EyeSide `json:"side"`
	WavelengthNM  float64 `json:"wavelength_nm"`  // e.g. 350 (UV), 450 (Blue), 520 (Green)
	ColorHex      string  `json:"color_hex"`      // Optional hex color such as "#8A2BE2" or "#FF4500"
	Intensity     float64 `json:"intensity"`      // [0.0, 1.0]
	LoomingRate   float64 `json:"looming_rate"`   // dOmega/dt expansion rate [0.0, 1.0]
	AzimuthDeg    float64 `json:"azimuth_deg"`    // Horizontal incidence angle (-180..+180, negative=left, positive=right)
	ElevationDeg  float64 `json:"elevation_deg"`  // Vertical incidence angle (-90..+90)
	DurationTicks int     `json:"duration_ticks"` // How many ticks the stimulus remains active
}

// SingleEyeState stores the physiological state of one compound eye hemisphere (~750 ommatidia).
type SingleEyeState struct {
	Adaptation     float64 `json:"adaptation"`      // Photoreceptor light adaptation level [0.0, 1.0]
	R1R6Luminance  float64 `json:"r1r6_luminance"`  // Outer photoreceptor broadband activation [0.0, 1.0]
	R7R8Chromatic  float64 `json:"r7r8_chromatic"`  // Inner photoreceptor UV/blue-green color activation [0.0, 1.0]
	SpectralLabel  string  `json:"spectral_label"`  // Dominant spectral band ("Dark", "UV-Dom", "Blue-Dom", "Green-Dom", "Broad")
	LoomingLevel   float64 `json:"looming_level"`   // Lobula looming collision activation [0.0, 1.0]
	LastWavelength float64 `json:"last_wavelength"` // Last perceived wavelength in nm
}

// CompoundEye belongs to the Domain Layer (Head Segment).
// Primary Responsibility: Transduce optical stimuli (wavelength/RGB, intensity, looming expansion)
// into visual pathway spikes (VIS_R1R6, VIS_R7R8, VIS_LO, VIS_LC) while maintaining photoreceptor adaptation.
type CompoundEye struct {
	Left             SingleEyeState  `json:"left"`
	Right            SingleEyeState  `json:"right"`
	AmbientLight     float64         `json:"ambient_light"`
	ActiveStimulus   *VisualStimulus `json:"active_stimulus,omitempty"`
	RemainingTicks   int             `json:"remaining_ticks"`
	VisualTurnBias   float64         `json:"visual_turn_bias"` // -1.0 (left) .. +1.0 (right)
	WasMovingLastTic bool            `json:"-"`
}

// NewCompoundEye constructs a CompoundEye initialized to normal daylight ambient conditions.
func NewCompoundEye() *CompoundEye {
	return &CompoundEye{
		Left: SingleEyeState{
			Adaptation:     0.20,
			R1R6Luminance:  0.65,
			R7R8Chromatic:  0.45,
			SpectralLabel:  "Broad-Daylight",
			LastWavelength: 500,
		},
		Right: SingleEyeState{
			Adaptation:     0.20,
			R1R6Luminance:  0.65,
			R7R8Chromatic:  0.45,
			SpectralLabel:  "Broad-Daylight",
			LastWavelength: 500,
		},
		AmbientLight: 0.85,
	}
}

func (e *CompoundEye) ID() string           { return "compound_eyes" }
func (e *CompoundEye) Segment() SegmentType { return SegmentHead }

// PerceiveLight assigns a visual perception task to the compound eyes.
func (e *CompoundEye) PerceiveLight(stim VisualStimulus) {
	stim.Intensity = clamp01(stim.Intensity)
	stim.LoomingRate = clamp01(stim.LoomingRate)
	if stim.WavelengthNM <= 0 && stim.ColorHex != "" {
		stim.WavelengthNM = rgbHexToApproxWavelength(stim.ColorHex)
	}
	if stim.WavelengthNM <= 0 {
		stim.WavelengthNM = 505 // Default blue-green peak
	}
	if stim.DurationTicks <= 0 {
		stim.DurationTicks = 1
	}
	if stim.Side == "" {
		stim.Side = EyeSideBoth
	}
	cp := stim
	e.ActiveStimulus = &cp
	e.RemainingTicks = stim.DurationTicks
}

// CollectAfferentSpikes advances photoreceptor adaptation and emits visual afferent spikes.
func (e *CompoundEye) CollectAfferentSpikes(dt time.Duration) []AfferentSpike {
	var spikes []AfferentSpike

	leftInt := e.AmbientLight * 0.65
	rightInt := e.AmbientLight * 0.65
	wavelength := 500.0
	leftLoom := 0.0
	rightLoom := 0.0

	if e.ActiveStimulus != nil && e.RemainingTicks > 0 {
		s := e.ActiveStimulus
		wavelength = s.WavelengthNM
		switch s.Side {
		case EyeSideLeft:
			leftInt = s.Intensity
			leftLoom = s.LoomingRate
		case EyeSideRight:
			rightInt = s.Intensity
			rightLoom = s.LoomingRate
		default:
			// Distribute by azimuth if provided
			azBias := clampRange(s.AzimuthDeg/90.0, -1.0, 1.0)
			leftInt = clamp01(s.Intensity * (1.0 - 0.35*math.Max(0, azBias)))
			rightInt = clamp01(s.Intensity * (1.0 + 0.35*math.Min(0, azBias)))
			leftLoom = s.LoomingRate
			rightLoom = s.LoomingRate
		}
		e.RemainingTicks--
		if e.RemainingTicks <= 0 {
			e.ActiveStimulus = nil
		}
	}

	// Update Left & Right eye states using Weber-Fechner adaptation
	e.updateSingleEye(&e.Left, leftInt, wavelength, leftLoom)
	e.updateSingleEye(&e.Right, rightInt, wavelength, rightLoom)

	// Compute bilateral visual steering bias:
	// Attractive chromatic signals bias toward the brighter/UV eye,
	// whereas strong looming biases away from the threatened eye.
	maxLoom := math.Max(e.Left.LoomingLevel, e.Right.LoomingLevel)
	if maxLoom > 0.3 {
		// Escape away from the looming side
		e.VisualTurnBias = clampRange(e.Left.LoomingLevel-e.Right.LoomingLevel, -1.0, 1.0)
	} else {
		// Phototaxis toward higher chromatic/luminance side
		e.VisualTurnBias = clampRange((e.Right.R7R8Chromatic-e.Left.R7R8Chromatic)*1.5+(e.Right.R1R6Luminance-e.Left.R1R6Luminance), -1.0, 1.0)
	}

	meanLum := 0.5 * (e.Left.R1R6Luminance + e.Right.R1R6Luminance)
	meanColor := 0.5 * (e.Left.R7R8Chromatic + e.Right.R7R8Chromatic)

	if meanLum > 0.2 {
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: e.ID(),
			TargetGroup: "VIS_R1R6",
			Scale:       meanLum,
			Pulses:      1,
		})
	}
	if meanColor > 0.15 {
		Pulses := 1
		if meanColor > 0.75 {
			Pulses = 2 // Strong UV/Blue-Green attraction
		}
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: e.ID(),
			TargetGroup: "VIS_R7R8",
			Scale:       meanColor,
			Pulses:      Pulses,
		})
	}
	if maxLoom > 0.25 {
		spikes = append(spikes,
			AfferentSpike{
				SourceOrgan: e.ID(),
				TargetGroup: "VIS_LO",
				Scale:       maxLoom * 1.4,
				Pulses:      2,
			},
			AfferentSpike{
				SourceOrgan: e.ID(),
				TargetGroup: "VIS_LC",
				Scale:       maxLoom * 1.6,
				Pulses:      2,
			},
		)
	}
	if e.WasMovingLastTic && meanLum > 0.1 {
		spikes = append(spikes, AfferentSpike{
			SourceOrgan: e.ID(),
			TargetGroup: "VIS_LPTC",
			Scale:       0.3,
			Pulses:      1,
		})
	}

	return spikes
}

func (e *CompoundEye) updateSingleEye(eye *SingleEyeState, intensity, wavelengthNM, looming float64) {
	// Photoreceptor adaptation slowly tracks input intensity
	eye.Adaptation = clamp01(eye.Adaptation*0.82 + intensity*0.18)
	effectiveGain := 1.0 - 0.35*eye.Adaptation

	eye.R1R6Luminance = clamp01(intensity * effectiveGain)
	spectralGain, label := drosophilaSpectralSensitivity(wavelengthNM, intensity)
	eye.R7R8Chromatic = clamp01(intensity * spectralGain * effectiveGain)
	eye.SpectralLabel = label
	eye.LastWavelength = wavelengthNM
	eye.LoomingLevel = clamp01(looming*0.85 + eye.LoomingLevel*0.15)
}

func (e *CompoundEye) Summary() string {
	return fmt.Sprintf(
		"Eye.L[Adapt=%.2f Lum=%.2f Color=%.2f(%s) Loom=%.2f] | Eye.R[Adapt=%.2f Lum=%.2f Color=%.2f(%s) Loom=%.2f]",
		e.Left.Adaptation, e.Left.R1R6Luminance, e.Left.R7R8Chromatic, e.Left.SpectralLabel, e.Left.LoomingLevel,
		e.Right.Adaptation, e.Right.R1R6Luminance, e.Right.R7R8Chromatic, e.Right.SpectralLabel, e.Right.LoomingLevel,
	)
}

// drosophilaSpectralSensitivity models R7 (UV ~350nm) and R8 (Blue ~450nm, Green ~520nm) sensitivity.
func drosophilaSpectralSensitivity(wavelengthNM, intensity float64) (float64, string) {
	if intensity < 0.1 {
		return 0.0, "Dark"
	}
	switch {
	case wavelengthNM >= 310 && wavelengthNM <= 395:
		return 1.25, "UV-Dom"
	case wavelengthNM > 395 && wavelengthNM <= 475:
		return 1.05, "Blue-Dom"
	case wavelengthNM > 475 && wavelengthNM <= 565:
		return 0.90, "Green-Dom"
	case wavelengthNM > 565:
		return 0.45, "Red-Weak"
	default:
		return 0.70, "Broad"
	}
}

func rgbHexToApproxWavelength(hex string) float64 {
	clean := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(clean) != 6 {
		return 500
	}
	r, err1 := strconv.ParseUint(clean[0:2], 16, 8)
	g, err2 := strconv.ParseUint(clean[2:4], 16, 8)
	b, err3 := strconv.ParseUint(clean[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return 500
	}
	// Violet/purple (#8A2BE2) maps to UV-Blue (~370nm)
	if b > 150 && r > 100 && g < 100 {
		return 365
	}
	if b >= r && b >= g {
		return 450
	}
	if g >= r && g >= b {
		return 520
	}
	return 610
}
