package valueobject

// AppConfig is the data/config.json document.
type AppConfig struct {
	LastProjectID      string `json:"lastProjectId"`
	ContextTokenBudget int    `json:"contextTokenBudget"`
}

// NormalizeConfig fills defaults for omitted values. A negative budget is
// invalid and is left untouched for validation by the store.
func NormalizeConfig(config AppConfig) AppConfig {
	if config.ContextTokenBudget == 0 {
		config.ContextTokenBudget = DefaultContextTokenBudget
	}
	return config
}
