package internal

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Muxcore-Media/core/pkg/contracts"
	"github.com/Muxcore-Media/scheduler-cron/internal/cronstore"
)

func (m *Module) Settings() []contracts.SettingDef {
	return m.settingsDefs()
}

func (m *Module) UpdateSetting(key, value string) error {
	return m.updateSetting(key, value)
}

func (m *Module) settingsDefs() []contracts.SettingDef {
	m.cfgMu.RLock()
	tz := m.tz
	storePath := m.storePath
	catchUp := m.catchUp
	m.cfgMu.RUnlock()
	return []contracts.SettingDef{
		{
			Key:         "timezone",
			Label:       "Timezone",
			Type:        contracts.SettingTypeString,
			Value:       tz,
			Default:     "UTC",
			Description: "IANA timezone for cron evaluation (SCHEDULER_TZ); live rebuild of the cron store",
			Group:       "Scheduling",
		},
		{
			Key:         "store_path",
			Label:       "Store Path",
			Type:        contracts.SettingTypeString,
			Value:       storePath,
			Default:     "",
			Description: "JSON persistence path (SCHEDULER_STORE_PATH); empty disables persistence",
			Group:       "Scheduling",
		},
		{
			Key:         "catch_up",
			Label:       "Missed-Fire Catch-Up",
			Type:        contracts.SettingTypeBool,
			Value:       strconv.FormatBool(catchUp),
			Default:     "true",
			Description: "Catch up missed fires on restore (SCHEDULER_CATCH_UP)",
			Group:       "Scheduling",
		},
	}
}

func (m *Module) updateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	m.cfgMu.RLock()
	tz := m.tz
	storePath := m.storePath
	catchUp := m.catchUp
	m.cfgMu.RUnlock()

	switch key {
	case "timezone", "SCHEDULER_TZ":
		if value == "" {
			value = "UTC"
		}
		if _, err := time.LoadLocation(value); err != nil {
			return fmt.Errorf("invalid timezone %q: %w", value, err)
		}
		tz = value
	case "store_path", "SCHEDULER_STORE_PATH":
		storePath = value
	case "catch_up", "SCHEDULER_CATCH_UP":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid catch_up %q", value)
		}
		catchUp = b
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return m.applySchedulerConfig(tz, storePath, catchUp)
}

func (m *Module) applySchedulerConfig(tz, storePath string, catchUp bool) error {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()

	tzChanged := tz != m.tz
	pathChanged := storePath != m.storePath
	catchChanged := catchUp != m.catchUp

	if !tzChanged && !pathChanged && !catchChanged {
		return nil
	}

	if m.srv == nil {
		m.tz = tz
		m.storePath = storePath
		m.catchUp = catchUp
		return nil
	}

	if tzChanged {
		store, err := cronstore.New(tz)
		if err != nil {
			return fmt.Errorf("init cron store: %w", err)
		}
		store.SetCatchUp(catchUp)
		if storePath != "" {
			store.EnablePersist(storePath)
			if err := store.Restore(m.srv.OnFire); err != nil {
				store.Stop()
				return fmt.Errorf("restore store %q: %w", storePath, err)
			}
		}
		old := m.srv.ReplaceStore(store)
		m.store = store
		m.tz = tz
		m.storePath = storePath
		m.catchUp = catchUp
		if old != nil {
			old.Stop()
		}
		return nil
	}

	if m.store != nil {
		if pathChanged {
			m.store.EnablePersist(storePath)
		}
		if catchChanged {
			m.store.SetCatchUp(catchUp)
		}
	}
	m.storePath = storePath
	m.catchUp = catchUp
	return nil
}
