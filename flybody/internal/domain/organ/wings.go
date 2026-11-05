package organ

import (
	"fmt"
	"time"
)

// WingPair belongs to the Domain Layer (Thorax Segment).
// Primary Responsibility: Actuate the left and right wings in response to MN_WING_L and MN_WING_R,
// producing aerodynamic flight thrust and climb velocity when wing motor neurons fire.
type WingPair struct {
	SpreadDeg    float64 `json:"spread_deg"`     // [0 = folded over abdomen, 75 = full flight spread]
	FrequencyHz  float64 `json:"frequency_hz"`   // Drosophila wingbeat ~180-220 Hz in flight
	ThrustMMPerS float64 `json:"thrust_mm_s"`    // Forward flight velocity contribution
	IsBeating    bool    `json:"is_beating"`
}

// NewWingPair constructs folded wings at rest.
func NewWingPair() *WingPair {
	return &WingPair{}
}

func (w *WingPair) ID() string           { return "wings_lr" }
func (w *WingPair) Segment() SegmentType { return SegmentThorax }

// ApplyEfferentDrive converts MN_WING_L/R activation into wingbeat frequency and flight thrust.
func (w *WingPair) ApplyEfferentDrive(drive MotorReadout, dt time.Duration) KinematicDelta {
	flightTotal := drive.FlightTotal()
	// Matching BEHAVIOR_THRESHOLDS.fly = 15 in fly-logic.js, while respecting grooming inhibition of flight
	if flightTotal > 15 && flightTotal > drive.GroomTotal() {
		intensity := clamp01(flightTotal / 28.0)
		w.IsBeating = true
		w.SpreadDeg = 65.0 + 15.0*intensity
		w.FrequencyHz = 185.0 + 35.0*intensity
		w.ThrustMMPerS = 22.0 + 28.0*intensity
		asymTurn := (drive.WingL - drive.WingR) * 4.0 + drive.LateralBias * 55.0

		return KinematicDelta{
			ForwardSpeedMMPerSec: w.ThrustMMPerS,
			YawRateDegPerSec:     asymTurn,
			ClimbRateMMPerSec:    4.5 * intensity,
			IsMoving:             true,
			IsFlying:             true,
			IsStartled:           drive.Startle > 30,
			EnergyExpended:       0.015 * (0.5 + 0.5*intensity),
		}
	}

	w.IsBeating = false
	w.SpreadDeg *= 0.3
	if w.SpreadDeg < 1.0 {
		w.SpreadDeg = 0
	}
	w.FrequencyHz = 0
	w.ThrustMMPerS = 0
	return KinematicDelta{}
}

func (w *WingPair) Summary() string {
	state := "FOLDED"
	if w.IsBeating {
		state = "FLIGHT_BEAT"
	}
	return fmt.Sprintf(
		"State=%s (Spread=%.1f° Freq=%.0fHz Thrust=%.1f mm/s)",
		state, w.SpreadDeg, w.FrequencyHz, w.ThrustMMPerS,
	)
}
