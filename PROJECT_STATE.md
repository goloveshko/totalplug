# 🚀 TotalPlug — Total Commander Plugin Manager & Updater

> **TotalPlug** — современная утилита на Go для автоматического аудита, проверки версий и обновления плагинов Total Commander (`WCX`, `WLX`, `WFX`, `WDX`).

---

## 🛠️ Стек технологий и Архитектура

* **Язык:** Go 1.26+
* **Архитектура:** Clean Architecture (изолированное ядро `/internal`, общее для CLI и GUI)
* **GUI-стек:** Wails v2 (Go Backend + WebView2, Windows 11 Mica / Тёмная тема)
* **Интерфейсы:** 
  * Единая точка входа с поддержкой **CLI-режима** (`cmd/totalplug`)
  * Полноценный **GUI-режим** (Wails)

---

## ✅ Что уже полностью реализовано и протестировано:

### 1. Сканер конфигурации Total Commander (`internal/config`)
* 🔍 **Поиск через реестр Windows:** Автоматическое обнаружение путей установки и `wincmd.ini` через ветки `HKCU` / `HKLM` (`SOFTWARE\Ghisler\Total Commander`, включая `WOW6432Node`).
* 📂 **Раскрытие виртуальных путей:** Автоматическая замена `%COMMANDER_PATH%` и `%COMMANDER_DRIVE%` на реальные пути установки.
* 🧹 **Фильтрация метаданных:** Игнорирование служебных ключей `_detect`, `_date`, `_flags`, `$checksum$`.
* 📦 **Умное именование и Дедупликация:**
  * Для архиваторов (WCX) имя плагина определяется по папке/бинарнику (например, `Total7zip` вместо расширений `7z`, `xz`, `bzip2`).
  * Дублирующиеся записи, указывающие на один и тот же файл плагина, объединяются в одну запись.

### 2. Четырёхуровневый извлекатель версий (`internal/scanner`)
* 🥇 **Tier 1 (PE-ресурсы):** Чтение `VS_FIXEDFILEINFO` и таблиц `StringFileInfo` (`FileVersion`, `ProductVersion`) через Windows API `version.dll`.
* 🥈 **Tier 2 (Companion DLLs):** Если основной файл не содержит ресурсов, сканер ищет соседние `.dll` библиотеки в папке (например, `MediaInfo64.dll`).
* 🥉 **Tier 3 (Текстовые документы):** Рекурсивный поиск файлов `pluginst.inf`, `Readme.txt`, `history.txt` в папке плагина и подпапках (`readme/`, `doc/`, `docs/`) с регулярными выражениями.
* 🏷️ **Tier 4 (Confidence / Метки доверия):** Каждой версии присваивается статус надежности: `[PE Binary]`, `[Readme/INF]`, `[Manual DB]`, `[Unknown]`.

### 3. Интеграция с онлайн-источниками и Умный матчер (`internal/checker`)
* 🌐 **Каталог `totalcmd.net`:** Загрузка полного списка плагинов через API `get_plugins_list.php` с **24-часовым локальным кешированием** на диске (`%APPDATA%\TotalPlug\totalcmd_catalog_cache.json`).
* 🐙 **GitHub Releases API:** Прямой опрос свежих тегов и прямых ссылок на `.zip` архивы для open-source плагинов (`Alexey-T/CudaLister`, `rweijnen/MDView`).
* 🔤 **Умная нормализация (`normalizeName`):**
  * Удаление префиксов (`TC`, `wfx_`, `wlx_`) и суффиксов (`64`, `32`, `plugin`).
  * Нормализация множественного числа (`Virtual Disks` ➔ `Virtual Disk`).
  * Встроенный словарь алиасов (`virtdisk`, `sftpplug`, `adbplugin` и др.).
* 🔢 **Семантическое сравнение (`CompareVersions`):**
  * Числовое покомпонентное сравнение (`5.2.0 == 5.2`, `1.20 > 1.2`).
  * Корректная обработка ситуаций, когда локальная версия новее каталожной.

### 4. Локальная база данных и Переопределения пользователя (`internal/storage`)
* 💾 **Пользовательские правки (`user_overrides.json`):** Возможность вручную переопределить для любого плагина имя, версию или кастомный источник (например, `github:Alexey-T/CudaLister`).
* 📝 **История обновлений (`installed_plugins.json`):** Фиксация установленных версий и хэшей файлов.

### 5. Консольный интерфейс CLI (`cmd/totalplug/main.go`)
* 📊 Группировка по категориям (`WCX`, `WLX`, `WFX`, `WDX`).
* 📐 Моноширинное табличное выравнивание с компактными эмодзи (`✓`, `🚀`, `⚠️`, `✗`) и обрезкой длинных имен (`truncateString`).
* 📋 Итоговая сводка (Summary) с легендой значков.
* ⚡ Флаг принудительного обновления кеша: `--refresh-cache` (или `-r`).

---

## 📁 Структура проекта

```text
totalplug/
├── cmd/
│   └── totalplug/
│       └── main.go           # CLI-интерфейс утилиты
├── internal/
│   ├── domain/
│   │   └── plugin.go         # Доменные сущности (Plugin, Status, Confidence, Override)
│   ├── config/
│   │   └── ini.go            # Поиск в реестре, парсинг wincmd.ini, дедупликация
│   ├── scanner/
│   │   └── version.go        # Извлечение версий через PE/DLL, текстовые файлы
│   ├── checker/
│   │   ├── checker.go        # Семантическое сравнение версий и сопоставление
│   │   ├── totalcmd.go       # Клиент к totalcmd.net API и 24h дисковый кеш
│   │   └── github.go         # Клиент к GitHub Releases API
│   └── storage/
│       └── db.go             # Работа с user_overrides.json и installed_plugins.json
├── frontend/                 # Wails UI (HTML / CSS / JS)
├── app.go                    # Wails адаптер к Go-сервисам
├── go.mod
├── go.sum
└── wails.json
```

---

## 🎯 План на следующие этапы:

1. **Репозиторий `totalplug-catalog`:**
   * Создание открытого community-каталога плагинов на GitHub с валидацией через JSON-Schema.
2. **Модуль Загрузчика и Обновлений (`internal/updater`):**
   * Скачивание `.zip` / `.rar` архивов по `DownloadURL`.
   * Безопасное резервное копирование старых файлов плагина (`.bak`).
   * Атомарная замена файлов плагина и обновление локальной БД.
3. **Графический интерфейс Wails GUI (`frontend/` + `app.go`):**
   * Перенос табличного вывода в красивый Fluent/Mica интерфейс с чекбоксами выбора плагинов и прогресс-баром.