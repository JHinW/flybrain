// Package brain belongs to the Domain Layer.
// It defines the BrainPort boundary interface and the built-in 59-group Drosophila
// functional connectome propagation engine (ConceptConnectome).
package brain

import "github.com/jhinw/flybrain/flybody/internal/domain/organ"

// BrainSnapshot captures the serializable membrane potentials and recent firing history of the brain.
type BrainSnapshot struct {
	EngineType       string             `json:"engine_type"`
	FireThreshold    float64            `json:"fire_threshold"`
	Potentials       map[string]float64 `json:"potentials"`
	LastFiredGroups  []string           `json:"last_fired_groups"`
	LastMotorReadout organ.MotorReadout `json:"last_motor_readout"`
}

// BrainPort is the Domain Layer port interface for the Drosophila brain.
// Iteration 1.0 uses ConceptConnectome (59 functional neuron groups);
// Iteration 1.2 can swap in CSRConnectome (139,255 FlyWire neurons) behind this exact interface.
type BrainPort interface {
	Propagate(spikes []organ.AfferentSpike, ambientLight float64) organ.MotorReadout
	ActiveGroups() []string
	TopPotentials(limit int) []NeuronPotential
	Snapshot() BrainSnapshot
	Restore(snap BrainSnapshot)
}

// NeuronPotential pairs a neuron group name with its current postsynaptic potential.
type NeuronPotential struct {
	Group     string  `json:"group"`
	Potential float64 `json:"potential"`
	Fired     bool    `json:"fired"`
}
