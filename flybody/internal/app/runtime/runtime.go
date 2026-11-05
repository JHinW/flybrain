// Package runtime belongs to the Application Layer.
// It orchestrates the 5-act physiological story of the virtual fruit fly across each tick:
// Sense -> Drive Evaluation -> Synaptic Propagation -> Effector Actuation -> Metabolic Settlement.
package runtime

import (
	"time"

	"github.com/jhinw/flybrain/flybody/internal/domain/body"
	"github.com/jhinw/flybrain/flybody/internal/domain/brain"
	"github.com/jhinw/flybrain/flybody/internal/domain/organ"
)

// DefaultTickDuration is 50ms (20 Hz physiological tick rate).
const DefaultTickDuration = 50 * time.Millisecond

// SoulContainerSnapshot is the complete persisted state of the VirtualBody and Brain.
type SoulContainerSnapshot struct {
	Version   string              `json:"version"`
	TickID    uint64              `json:"tick_id"`
	UpdatedAt string              `json:"updated_at"`
	Body      *body.VirtualBody   `json:"body"`
	Brain     brain.BrainSnapshot `json:"brain"`
}

// SnapshotRepository is the Application Layer persistence port implemented by Infrastructure stores.
type SnapshotRepository interface {
	Save(path string, snap SoulContainerSnapshot) error
	Load(path string) (SoulContainerSnapshot, error)
}

// TickReport captures the narrative events of a single physiological tick.
type TickReport struct {
	TickID         uint64                `json:"tick_id"`
	Behavior       body.BehaviorMode     `json:"behavior"`
	AfferentSpikes []organ.AfferentSpike `json:"afferent_spikes"`
	FiredGroups    []string              `json:"fired_groups"`
	MotorReadout   organ.MotorReadout    `json:"motor_readout"`
	Kinematics     organ.KinematicDelta  `json:"kinematics"`
}

// OrganDiffItem records a single organ's before/after summary across a multi-tick run.
type OrganDiffItem struct {
	Segment string `json:"segment"`
	OrganID string `json:"organ_id"`
	Before  string `json:"before"`
	After   string `json:"after"`
	Changed bool   `json:"changed"`
}

// TransitionSummary compares the fly's state before and after a stimulus or step sequence.
type TransitionSummary struct {
	StartTick      uint64          `json:"start_tick"`
	EndTick        uint64          `json:"end_tick"`
	BeforeBehavior body.BehaviorMode `json:"before_behavior"`
	AfterBehavior  body.BehaviorMode `json:"after_behavior"`
	BeforePose     body.Pose3D     `json:"before_pose"`
	AfterPose      body.Pose3D     `json:"after_pose"`
	OrganDiffs     []OrganDiffItem `json:"organ_diffs"`
	TickReports    []TickReport    `json:"tick_reports"`
}

// PhysiologyRuntime belongs to the Application Layer.
// Primary Responsibility: Orchestrate the life cycle ticks of VirtualBody and BrainPort
// like a story, track logical TickID, and coordinate state snapshots.
type PhysiologyRuntime struct {
	tickID uint64
	body   *body.VirtualBody
	brain  brain.BrainPort
	store  SnapshotRepository
}

// NewPhysiologyRuntime creates a PhysiologyRuntime binding a VirtualBody, BrainPort, and SnapshotRepository.
func NewPhysiologyRuntime(b *body.VirtualBody, br brain.BrainPort, st SnapshotRepository) *PhysiologyRuntime {
	return &PhysiologyRuntime{
		tickID: 0,
		body:   b,
		brain:  br,
		store:  st,
	}
}

func (rt *PhysiologyRuntime) TickID() uint64          { return rt.tickID }
func (rt *PhysiologyRuntime) Body() *body.VirtualBody { return rt.body }
func (rt *PhysiologyRuntime) Brain() brain.BrainPort  { return rt.brain }

// StepOneTick executes the 5-Act physiological story for a single tick.
func (rt *PhysiologyRuntime) StepOneTick(dt time.Duration) TickReport {
	if dt <= 0 {
		dt = DefaultTickDuration
	}

	// Act I: Sensory Organs Adapt & Transduce External Stimuli into Afferent Spikes
	afferentSpikes := rt.body.CollectAfferentSpikes(dt)

	// Act II: Metabolic Tissue Evaluates Homeostasis & Emits Drive Pulses
	driveSpikes := rt.body.Metabolism.CollectDriveSpikes(rt.body.SensationSummary(), dt)
	allSpikes := append(afferentSpikes, driveSpikes...)

	// Act III: Brain Propagates Synaptic Currents & Drains Motor Readout
	motorReadout := rt.brain.Propagate(allSpikes, rt.body.Eyes.AmbientLight)
	motorReadout.LateralBias = rt.body.CombinedLateralBias()

	// Act IV: Effector Organs Actuate Muscles & Update Posture / Kinematics
	kinematics := rt.body.ActuateEffectors(motorReadout, dt)

	// Act V: Metabolism Settles Energy Expenditure & Body Integrates Local 3D Motion
	rt.body.SettleTick(kinematics, dt)
	rt.tickID++

	return TickReport{
		TickID:         rt.tickID,
		Behavior:       rt.body.Behavior,
		AfferentSpikes: allSpikes,
		FiredGroups:    rt.brain.ActiveGroups(),
		MotorReadout:   motorReadout,
		Kinematics:     kinematics,
	}
}

// StepTicks advances N physiological ticks and returns a before/after TransitionSummary.
func (rt *PhysiologyRuntime) StepTicks(ticks int, dt time.Duration) TransitionSummary {
	if ticks <= 0 {
		ticks = 1
	}
	startTick := rt.tickID
	beforeBehavior := rt.body.Behavior
	beforePose := rt.body.Pose
	beforeSummaries := rt.captureOrganSummaries()

	reports := make([]TickReport, 0, ticks)
	for i := 0; i < ticks; i++ {
		reports = append(reports, rt.StepOneTick(dt))
	}

	afterSummaries := rt.captureOrganSummaries()
	diffs := make([]OrganDiffItem, len(beforeSummaries))
	for i := range beforeSummaries {
		diffs[i] = OrganDiffItem{
			Segment: beforeSummaries[i].segment,
			OrganID: beforeSummaries[i].id,
			Before:  beforeSummaries[i].summary,
			After:   afterSummaries[i].summary,
			Changed: beforeSummaries[i].summary != afterSummaries[i].summary,
		}
	}

	return TransitionSummary{
		StartTick:      startTick,
		EndTick:        rt.tickID,
		BeforeBehavior: beforeBehavior,
		AfterBehavior:  rt.body.Behavior,
		BeforePose:     beforePose,
		AfterPose:      rt.body.Pose,
		OrganDiffs:     diffs,
		TickReports:    reports,
	}
}

type organSnapSummary struct {
	segment string
	id      string
	summary string
}

func (rt *PhysiologyRuntime) captureOrganSummaries() []organSnapSummary {
	organs := []organ.Organ{
		rt.body.Eyes,
		rt.body.Antennae,
		rt.body.Proboscis,
		rt.body.Neck,
		rt.body.Legs,
		rt.body.Wings,
		rt.body.Bristles,
		rt.body.Metabolism,
	}
	out := make([]organSnapSummary, len(organs))
	for i, o := range organs {
		out[i] = organSnapSummary{
			segment: string(o.Segment()),
			id:      o.ID(),
			summary: o.Summary(),
		}
	}
	return out
}

// ExportSnapshot packages the current runtime state into a SoulContainerSnapshot.
func (rt *PhysiologyRuntime) ExportSnapshot() SoulContainerSnapshot {
	return SoulContainerSnapshot{
		Version:   "flysoul/v1.0",
		TickID:    rt.tickID,
		UpdatedAt: time.Now().Format(time.RFC3339),
		Body:      rt.body,
		Brain:     rt.brain.Snapshot(),
	}
}

// RestoreSnapshot restores the VirtualBody, Brain, and TickID from a SoulContainerSnapshot.
func (rt *PhysiologyRuntime) RestoreSnapshot(snap SoulContainerSnapshot) {
	rt.tickID = snap.TickID
	if snap.Body != nil {
		rt.body = snap.Body
	}
	rt.brain.Restore(snap.Brain)
}

// SaveToFile persists the current state to disk via the configured SnapshotRepository.
func (rt *PhysiologyRuntime) SaveToFile(path string) error {
	if rt.store == nil || path == "" {
		return nil
	}
	return rt.store.Save(path, rt.ExportSnapshot())
}

// LoadFromFile restores state from disk via the configured SnapshotRepository.
func (rt *PhysiologyRuntime) LoadFromFile(path string) error {
	if rt.store == nil || path == "" {
		return nil
	}
	snap, err := rt.store.Load(path)
	if err != nil {
		return err
	}
	rt.RestoreSnapshot(snap)
	return nil
}
