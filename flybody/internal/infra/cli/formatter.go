// Package cli belongs to the Infrastructure Layer.
// It handles command-line argument parsing, interactive REPL execution, and
// human/machine-readable state formatting for the VirtualBody.
package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jhinw/flybrain/flybody/internal/app/runtime"
)

// FormatBodyTree renders the full anatomical hierarchy of the virtual body, active brain regions,
// and origin-free local 3D pose in the ASCII tree format defined in the architecture specification.
func FormatBodyTree(rt *runtime.PhysiologyRuntime) string {
	b := rt.Body()
	br := rt.Brain()
	var sb strings.Builder

	fmt.Fprintf(&sb, "[FlyNode: %s] Tick: #%d | Behavior: %s | Glycogen: %.1f%% | ATP: %.1f%%\n",
		b.NodeID, rt.TickID(), b.Behavior, b.Metabolism.GlycogenReserve*100, b.Metabolism.ATPLevel*100)
	fmt.Fprintf(&sb, "├── POSE_3D (Local Relational Frame: Δx=%+.2fmm, Δy=%+.2fmm, Δz=%+.2fmm | Heading=%.1f°)\n",
		b.Pose.DispXMM, b.Pose.DispYMM, b.Pose.DispZMM, b.Pose.HeadingDeg)

	fmt.Fprintf(&sb, "├── HEAD (Pitch: %+.1f°, Yaw: %+.1f°)\n", b.Neck.PitchDeg, b.Neck.YawDeg)
	fmt.Fprintf(&sb, "│   ├── Eye.Left   : Adapt=%.2f | R1-R6(Lum)=%.2f | R7/R8(Color)=%.2f(%s) | Looming=%.2f\n",
		b.Eyes.Left.Adaptation, b.Eyes.Left.R1R6Luminance, b.Eyes.Left.R7R8Chromatic, b.Eyes.Left.SpectralLabel, b.Eyes.Left.LoomingLevel)
	fmt.Fprintf(&sb, "│   ├── Eye.Right  : Adapt=%.2f | R1-R6(Lum)=%.2f | R7/R8(Color)=%.2f(%s) | Looming=%.2f\n",
		b.Eyes.Right.Adaptation, b.Eyes.Right.R1R6Luminance, b.Eyes.Right.R7R8Chromatic, b.Eyes.Right.SpectralLabel, b.Eyes.Right.LoomingLevel)
	fmt.Fprintf(&sb, "│   ├── Antenna.L/R: %s\n", b.Antennae.Summary())
	fmt.Fprintf(&sb, "│   └── Proboscis  : %s\n", b.Proboscis.Summary())

	fmt.Fprintf(&sb, "├── THORAX (Speed: %.2f mm/s, YawRate: %+.1f°/s, TripodGaitPhase: %.0f°)\n",
		b.Legs.WalkingSpeedMM, b.Legs.YawRateDeg, b.Legs.TripodPhaseDeg)
	fmt.Fprintf(&sb, "│   ├── Tripod.A (L1, R2, L3): %-13s (PushForce=%.2f, CoxaAngle=%+.0f°, Lift=%.2fmm)\n",
		b.Legs.TripodA.PhaseLabel, b.Legs.TripodA.PushForce, b.Legs.TripodA.CoxaAngleDeg, b.Legs.TripodA.LiftHeightMM)
	fmt.Fprintf(&sb, "│   ├── Tripod.B (R1, L2, R3): %-13s (PushForce=%.2f, CoxaAngle=%+.0f°, Lift=%.2fmm)\n",
		b.Legs.TripodB.PhaseLabel, b.Legs.TripodB.PushForce, b.Legs.TripodB.CoxaAngleDeg, b.Legs.TripodB.LiftHeightMM)
	fmt.Fprintf(&sb, "│   └── Wings (L/R)          : %s\n", b.Wings.Summary())

	fmt.Fprintf(&sb, "├── ABDOMEN\n")
	fmt.Fprintf(&sb, "│   ├── Bristles             : %s\n", b.Bristles.Summary())
	fmt.Fprintf(&sb, "│   └── Metabolism           : Hunger=%.2f | Fear=%.2f | Fatigue=%.2f | Curiosity=%.2f | GroomUrge=%.2f\n",
		b.Metabolism.Drives.Hunger, b.Metabolism.Drives.Fear, b.Metabolism.Drives.Fatigue,
		b.Metabolism.Drives.Curiosity, b.Metabolism.Drives.Groom)

	activeGroups := br.ActiveGroups()
	activeStr := "none"
	if len(activeGroups) > 0 {
		activeStr = strings.Join(activeGroups, ", ")
	}
	topPots := br.TopPotentials(6)
	var potParts []string
	for _, p := range topPots {
		potParts = append(potParts, fmt.Sprintf("%s=%.0f", p.Group, p.Potential))
	}
	potStr := "baseline"
	if len(potParts) > 0 {
		potStr = strings.Join(potParts, ", ")
	}
	fmt.Fprintf(&sb, "└── BRAIN (59-Group Connectome)\n")
	fmt.Fprintf(&sb, "    ├── Fired/Active Groups  : [%s]\n", activeStr)
	fmt.Fprintf(&sb, "    └── Top Membrane Pots    : %s\n", potStr)

	return sb.String()
}

// FormatTransition renders the before/after stimulation diff and the updated body tree.
func FormatTransition(rt *runtime.PhysiologyRuntime, tr runtime.TransitionSummary) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== Physiological Story Transition (Tick #%d -> #%d) ===\n", tr.StartTick, tr.EndTick)
	fmt.Fprintf(&sb, "  Behavior : %s  -->  %s\n", tr.BeforeBehavior, tr.AfterBehavior)
	fmt.Fprintf(&sb, "  Pose 3D  : (%.2f, %.2f, %.2f mm @ %.1f°)  -->  (%.2f, %.2f, %.2f mm @ %.1f°)\n",
		tr.BeforePose.DispXMM, tr.BeforePose.DispYMM, tr.BeforePose.DispZMM, tr.BeforePose.HeadingDeg,
		tr.AfterPose.DispXMM, tr.AfterPose.DispYMM, tr.AfterPose.DispZMM, tr.AfterPose.HeadingDeg)

	fmt.Fprintf(&sb, "\n--- Organ & Tissue State Changes ---\n")
	changedCount := 0
	for _, d := range tr.OrganDiffs {
		if d.Changed {
			changedCount++
			fmt.Fprintf(&sb, "  * [%s / %s]\n", d.Segment, d.OrganID)
			fmt.Fprintf(&sb, "      Before: %s\n", d.Before)
			fmt.Fprintf(&sb, "      After : %s\n", d.After)
		}
	}
	if changedCount == 0 {
		fmt.Fprintf(&sb, "  (No organ state changes)\n")
	}

	if len(tr.TickReports) > 0 {
		last := tr.TickReports[len(tr.TickReports)-1]
		fmt.Fprintf(&sb, "\n--- Final Tick (#%d) Neural & Motor Readout ---\n", last.TickID)
		fmt.Fprintf(&sb, "  Fired Groups : %v\n", last.FiredGroups)
		fmt.Fprintf(&sb, "  Motor Output : WalkL=%.0f WalkR=%.0f Flight=%.0f Feed=%.0f Groom=%.0f Startle=%.0f Head=%.0f\n",
			last.MotorReadout.WalkLeftTotal(), last.MotorReadout.WalkRightTotal(),
			last.MotorReadout.FlightTotal(), last.MotorReadout.Proboscis,
			last.MotorReadout.GroomTotal(), last.MotorReadout.Startle, last.MotorReadout.Head)
	}

	fmt.Fprintf(&sb, "\n--- Current Virtual Body State Tree ---\n")
	sb.WriteString(FormatBodyTree(rt))
	return sb.String()
}

// FormatBodyJSON serializes the current runtime snapshot as pretty JSON.
func FormatBodyJSON(rt *runtime.PhysiologyRuntime) (string, error) {
	data, err := json.MarshalIndent(rt.ExportSnapshot(), "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
