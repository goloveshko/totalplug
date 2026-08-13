package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"totalplug/internal/domain"

	"gopkg.in/ini.v1"
)

// FindDefaultIniPath пытается найти wincmd.ini в стандартных местах Windows
func FindDefaultIniPath(tcDir string) string {
	// 1. Проверяем в самой папке Total Commander
	if tcDir != "" {
		localIni := filepath.Join(tcDir, "wincmd.ini")
		if _, err := os.Stat(localIni); err == nil {
			return localIni
		}
	}

	// 2. Проверяем в %APPDATA%\GHISLER\wincmd.ini
	appData := os.Getenv("APPDATA")
	if appData != "" {
		appDataIni := filepath.Join(appData, "GHISLER", "wincmd.ini")
		if _, err := os.Stat(appDataIni); err == nil {
			return appDataIni
		}
	}

	// 3. Проверяем в C:\Windows\wincmd.ini
	winDir := os.Getenv("SystemRoot")
	if winDir != "" {
		winIni := filepath.Join(winDir, "wincmd.ini")
		if _, err := os.Stat(winIni); err == nil {
			return winIni
		}
	}

	return ""
}

// ParsePluginsFromIni парсит wincmd.ini и возвращает список найденных плагинов
func ParsePluginsFromIni(iniPath string) ([]domain.Plugin, error) {
	cfg, err := ini.Load(iniPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ini файла: %w", err)
	}

	var plugins []domain.Plugin

	// Вспомогательная функция парсинга секции
	parseSection := func(sectionName string, pType domain.PluginType) {
		sec := cfg.Section(sectionName)
		if sec == nil {
			return
		}

		for _, key := range sec.Keys() {
			val := key.Value()
			if val == "" {
				continue
			}

			// Формат записи в INI часто равен "флаги,путь_к_файлу" (например: 223,C:\plugins\7zip.wcx)
			pluginPath := val
			if idx := strings.Index(val, ","); idx != -1 {
				pluginPath = val[idx+1:]
			}
			pluginPath = strings.TrimSpace(pluginPath)

			// Заменяем переменные окружения типа %COMMANDER_PATH% на реальный путь
			pluginPath = os.ExpandEnv(pluginPath)

			plugin := domain.Plugin{
				ID:               fmt.Sprintf("%s_%s", pType, key.Name()),
				Name:             key.Name(),
				Type:             pType,
				Path:             pluginPath,
				InstalledVersion: "Не определена", // Добавим чтение версии из PE позже
				Status:           domain.StatusUpToDate,
			}

			// Проверяем, существует ли файл плагина на диске
			if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
				plugin.Status = domain.StatusNotFound
			}

			plugins = append(plugins, plugin)
		}
	}

	// Читаем все 4 типа плагинов TC
	parseSection("PackerPlugins", domain.TypePacker)
	parseSection("ListerPlugins", domain.TypeLister)
	parseSection("FileSystemPlugins", domain.TypeFileSystem)
	parseSection("ContentPlugins", domain.TypeContent)

	return plugins, nil
}
