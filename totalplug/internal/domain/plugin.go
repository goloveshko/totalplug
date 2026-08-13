package domain

import "time"

// PluginType описывает тип плагина Total Commander
type PluginType string

const (
	TypePacker     PluginType = "WCX" // Архиваторные плагины
	TypeLister     PluginType = "WLX" // Плагины просмотра
	TypeFileSystem PluginType = "WFX" // Плагины файловой системы
	TypeContent    PluginType = "WDX" // Информационные плагины
)

// PluginStatus отражает текущий статус плагина
type PluginStatus string

const (
	StatusUpToDate    PluginStatus = "up_to_date"    // Обновления не требуются
	StatusUpdateAvail PluginStatus = "update_avail" // Доступна новая версия
	StatusNotFound    PluginStatus = "not_found"    // Файл плагина не найден на диске
	StatusError       PluginStatus = "error"        // Ошибка проверки
)

// Plugin — главная доменная модель плагина
type Plugin struct {
	ID               string       `json:"id"`                // Уникальный идентификатор
	Name             string       `json:"name"`              // Имя плагина
	Type             PluginType   `json:"type"`              // Тип плагина (WCX/WLX/WFX/WDX)
	Path             string       `json:"path"`              // Абсолютный путь к файлу .wcx/.wlx
	InstalledVersion string       `json:"installed_version"` // Текущая версия
	LatestVersion    string       `json:"latest_version"`    // Доступная версия в сети
	DownloadURL      string       `json:"download_url"`      // Ссылка на скачивание zip/rar
	Status           PluginStatus `json:"status"`            // Статус
	LastChecked      time.Time    `json:"last_checked"`      // Дата последней проверки
}

// Config содержит конфигурацию поиска Total Commander
type Config struct {
	TCPath    string `json:"tc_path"`    // Путь к папке Total Commander
	IniPath   string `json:"ini_path"`   // Путь к wincmd.ini
	AutoCheck bool   `json:"auto_check"` // Автопроверка при старте
}
