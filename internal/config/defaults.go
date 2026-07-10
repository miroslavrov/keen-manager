package config

import "github.com/miroslavrov/keen-manager/internal/model"

// DefaultState returns a fresh, safe configuration for a new install.
func DefaultState() model.State {
	return model.State{
		Version:     SchemaVersion,
		Connections: []model.Connection{},
		Subscriptions: []model.Subscription{},
		Failover: model.Failover{
			Enabled:          false,
			Chain:            []string{},
			CheckIntervalS:   30,
			FailureThreshold: 3,
			AutoReturn:       true,
			ProbeTarget:      "https://www.gstatic.com/generate_204",
		},
		Settings: model.Settings{
			Port:                  47115,
			AuthEnabled:           false,
			Theme:                 "dark",
			BackupOnChange:        true,
			RollbackTimeoutS:      90,
			KillSwitchDefault:     false,
			AutoSelectIntervalMin: 30,
		},
	}
}

// migrate upgrades an older State in place.
func migrate(s *model.State) {
	// Capture the on-disk schema version BEFORE we touch it, so version-gated
	// migrations below see the real "from" (a v0/v1 state, or a fresh 0).
	from := s.Version

	if s.Settings.Port == 0 {
		s.Settings.Port = 47115
	}
	if s.Failover.CheckIntervalS == 0 {
		s.Failover.CheckIntervalS = 30
	}
	if s.Failover.FailureThreshold == 0 {
		s.Failover.FailureThreshold = 3
	}
	if s.Failover.ProbeTarget == "" {
		s.Failover.ProbeTarget = "https://www.gstatic.com/generate_204"
	}
	if s.Connections == nil {
		s.Connections = []model.Connection{}
	}
	if s.Subscriptions == nil {
		s.Subscriptions = []model.Subscription{}
	}

	// Schema v2: Subscription.Enabled was added. Pre-v2 state has no such field,
	// so it unmarshals to false — but every existing subscription was implicitly
	// active. Enable them all on upgrade so a migration never silently kills a
	// user's fleet. New subscriptions are created enabled (see CreateSubscription).
	if from < 2 {
		for i := range s.Subscriptions {
			s.Subscriptions[i].Enabled = true
		}
	}

	s.Version = SchemaVersion
}
