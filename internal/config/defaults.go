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
			Port:                  8088,
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
	if s.Version == 0 {
		s.Version = SchemaVersion
	}
	if s.Settings.Port == 0 {
		s.Settings.Port = 8088
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
}
