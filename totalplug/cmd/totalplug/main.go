package main

import (
	"fmt"
	"os"

	"totalplug/internal/config"
	"totalplug/internal/domain"
)

func main() {
	fmt.Println("==========================================")
	fmt.Println("   TotalPlug - Total Commander Updater    ")
	fmt.Println("==========================================")

	// 1. Locate wincmd.ini automatically
	iniPath, tcDir := config.FindDefaultIniPath("")

	if iniPath == "" {
		fmt.Println("[!] Error: Could not automatically locate wincmd.ini via Registry or AppData.")
		fmt.Println("[!] Please specify the path to wincmd.ini manually.")
		os.Exit(1)
	}

	fmt.Printf("[+] Total Commander Dir: %s\n", tcDir)
	fmt.Printf("[+] Found wincmd.ini:    %s\n", iniPath)

	// 2. Parse plugins from INI
	plugins, err := config.ParsePluginsFromIni(iniPath, tcDir)
	if err != nil {
		fmt.Printf("[!] Failed to parse plugins: %v\n", err)
		os.Exit(1)
	}

	if len(plugins) == 0 {
		fmt.Println("[i] No plugins found in configuration.")
		return
	}

	// Categories list for structured output
	categories := []struct {
		Type  domain.PluginType
		Title string
	}{
		{Type: domain.TypePacker, Title: "Packer / Archiver Plugins (WCX)"},
		{Type: domain.TypeLister, Title: "Lister / Viewer Plugins (WLX)"},
		{Type: domain.TypeFileSystem, Title: "File System Plugins (WFX)"},
		{Type: domain.TypeContent, Title: "Content / Meta Plugins (WDX)"},
	}

	var validCount, missingCount int
	itemIndex := 1

	fmt.Printf("\nDiscovered %d installed plugins:\n", len(plugins))

	// 3. Render plugins grouped by category
	for _, cat := range categories {
		var group []domain.Plugin
		for _, p := range plugins {
			if p.Type == cat.Type {
				group = append(group, p)
			}
		}

		if len(group) == 0 {
			continue
		}

		fmt.Printf("\n--- [%s] %s (%d) ---\n", cat.Type, cat.Title, len(group))

		for _, p := range group {
			statusSymbol := "✓"
			if p.Status == domain.StatusNotFound {
				statusSymbol = "✗ [File Missing]"
				missingCount++
			} else {
				validCount++
			}

			fmt.Printf("%2d. %-20s %-16s -> %s\n",
				itemIndex, p.Name, statusSymbol, p.Path)
			itemIndex++
		}
	}

	// 4. Print Summary Status Footer
	fmt.Println("\n================================================================================")
	fmt.Printf("SUMMARY: Total %d plugins | %d Valid ✓ | %d Missing ✗\n",
		len(plugins), validCount, missingCount)

	if missingCount > 0 {
		fmt.Printf("[!] WARNING: %d plugin file(s) configured in wincmd.ini were not found on disk!\n", missingCount)
	} else {
		fmt.Println("[+] SUCCESS: All configured plugin files exist on disk.")
	}
	fmt.Println("================================================================================")
}
