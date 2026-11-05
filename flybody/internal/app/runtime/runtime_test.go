package runtime_test

import (
	"path/filepath"
	"testing"

	"github.com/jhinw/flybrain/flybody/internal/app/runtime"
	"github.com/jhinw/flybrain/flybody/internal/domain/body"
	"github.com/jhinw/flybrain/flybody/internal/domain/brain"
	"github.com/jhinw/flybrain/flybody/internal/domain/organ"
	"github.com/jhinw/flybrain/flybody/internal/infra/store"
)

func newTestRuntime() *runtime.PhysiologyRuntime {
	b := body.NewVirtualBody("test-fly-01")
	br := brain.NewConceptConnectome()
	st := store.NewJSONSnapshotStore()
	return runtime.NewPhysiologyRuntime(b, br, st)
}

func TestVisualLoomingEscapeReflex(t *testing.T) {
	rt := newTestRuntime()

	// Stimulate compound eyes with a rapidly looming object
	rt.Body().Eyes.PerceiveLight(organ.VisualStimulus{
		Side:          organ.EyeSideBoth,
		ColorHex:      "#FF4500",
		Intensity:     0.9,
		LoomingRate:   0.85,
		DurationTicks: 4,
	})

	tr := rt.StepTicks(4, runtime.DefaultTickDuration)

	if !rt.Body().Wings.IsBeating && !rt.Body().Legs.JumpingActive {
		t.Fatalf("expected looming stimulus to trigger wing flight or escape jump, got behavior=%s", tr.AfterBehavior)
	}
	if tr.AfterBehavior != body.BehaviorEscapeJump && tr.AfterBehavior != body.BehaviorFlight {
		t.Fatalf("expected ESCAPE_JUMP or FLIGHT_CRUISE behavior, got %s", tr.AfterBehavior)
	}
}

func TestStereoChemotaxisAndSweetFeedingLoop(t *testing.T) {
	rt := newTestRuntime()

	// 1. Stimulate right antenna more strongly than left with food odor
	rt.Body().Antennae.PerceiveAntennal(organ.AntennalStimulus{
		OdorType:      organ.OdorFood,
		LeftOdorConc:  0.25,
		RightOdorConc: 0.85,
		DurationTicks: 5,
	})

	trWalk := rt.StepTicks(5, runtime.DefaultTickDuration)
	if trWalk.AfterBehavior != body.BehaviorWalkApproach && trWalk.AfterBehavior != body.BehaviorWalkExplore {
		t.Fatalf("expected walking behavior during food chemotaxis, got %s", trWalk.AfterBehavior)
	}
	if rt.Body().Antennae.StereoOdorBias <= 0 {
		t.Fatalf("expected positive right-ward stereo odor bias, got %f", rt.Body().Antennae.StereoOdorBias)
	}

	// 2. Contact sweet sugar with proboscis -> should extend proboscis and reduce hunger
	hungerBefore := rt.Body().Metabolism.Drives.Hunger
	rt.Body().Proboscis.PerceiveTaste(organ.GustatoryStimulus{
		Taste:         organ.TasteSweet,
		Concentration: 0.90,
		DurationTicks: 4,
	})

	trFeed := rt.StepTicks(4, runtime.DefaultTickDuration)
	if trFeed.AfterBehavior != body.BehaviorFeed {
		t.Fatalf("expected FEED_PROBOSCIS behavior on sweet taste contact, got %s", trFeed.AfterBehavior)
	}
	if rt.Body().Proboscis.Extension <= 0.35 {
		t.Fatalf("expected proboscis extension > 0.35, got %f", rt.Body().Proboscis.Extension)
	}
	if rt.Body().Metabolism.Drives.Hunger >= hungerBefore {
		t.Fatalf("expected hunger drive to decrease after feeding (before=%f, after=%f)",
			hungerBefore, rt.Body().Metabolism.Drives.Hunger)
	}
}

func TestThoracicTouchGroomingReflex(t *testing.T) {
	rt := newTestRuntime()

	// Stimulate thorax bristles with moderate mechanical touch
	rt.Body().Bristles.PerceiveTouch(organ.TouchStimulus{
		TargetLocation: "thorax",
		Force:          0.65,
		DurationTicks:  4,
	})

	tr := rt.StepTicks(4, runtime.DefaultTickDuration)
	if tr.AfterBehavior != body.BehaviorGroom && !rt.Body().Legs.GroomingActive {
		t.Fatalf("expected GROOM_CLEAN behavior after thoracic bristle touch, got %s", tr.AfterBehavior)
	}
}

func TestSnapshotSaveAndRestoreRoundTrip(t *testing.T) {
	rt1 := newTestRuntime()
	rt1.Body().Eyes.PerceiveLight(organ.VisualStimulus{
		Side:          organ.EyeSideLeft,
		WavelengthNM:  365,
		Intensity:     0.85,
		DurationTicks: 3,
	})
	rt1.StepTicks(3, runtime.DefaultTickDuration)

	tmpFile := filepath.Join(t.TempDir(), "test.flysoul.json")
	if err := rt1.SaveToFile(tmpFile); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	rt2 := newTestRuntime()
	if err := rt2.LoadFromFile(tmpFile); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if rt2.TickID() != rt1.TickID() {
		t.Fatalf("expected restored TickID %d, got %d", rt1.TickID(), rt2.TickID())
	}
	if rt2.Body().Behavior != rt1.Body().Behavior {
		t.Fatalf("expected restored Behavior %s, got %s", rt1.Body().Behavior, rt2.Body().Behavior)
	}
	if rt2.Body().Eyes.Left.SpectralLabel != "UV-Dom" {
		t.Fatalf("expected restored Left Eye SpectralLabel 'UV-Dom', got %q", rt2.Body().Eyes.Left.SpectralLabel)
	}
}
