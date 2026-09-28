package system

import (
	"encoding/json"
	"errors"

	"github.com/kaoqy/Nodeloc-Store/internal/models"
	"gorm.io/gorm"
)

// SettingKey is the AppSetting row that carries the runtime configuration.
const SettingKey = "runtime.config"

// LoadRuntime reads the runtime config from the database. It returns nil when
// the wizard has not stored anything yet.
func LoadRuntime(db *gorm.DB) (*RuntimeConfig, error) {
	var row models.AppSetting
	err := db.Where("key = ?", SettingKey).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.Value == nil || *row.Value == "" {
		return nil, nil
	}
	var rt RuntimeConfig
	if err := json.Unmarshal([]byte(*row.Value), &rt); err != nil {
		return nil, err
	}
	rt.MergeDefaults()
	rt.Normalize()
	return &rt, nil
}

// SaveRuntime persists the runtime config.
func SaveRuntime(db *gorm.DB, rt *RuntimeConfig) error {
	data, err := json.Marshal(rt)
	if err != nil {
		return err
	}
	value := string(data)
	var row models.AppSetting
	result := db.Where("key = ?", SettingKey).First(&row)
	if result.Error != nil {
		if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return result.Error
		}
		return db.Create(&models.AppSetting{Key: SettingKey, Value: &value}).Error
	}
	row.Value = &value
	return db.Save(&row).Error
}
