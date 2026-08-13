package main

import (
	"fmt"
	"os"

	"totalplug/internal/config"
	"totalplug/internal/domain"
	"totalplug/internal/scanner"
)

func main() {
	fmt.Println("==========================================")
	fmt.Println("   TotalPlug - Total Commander Updater    ")
	fmt.Println("==========================================")

	iniPath, tcDir := config.FindDefaultIniPath("")
	if iniPath == "" {
		fmt.Println("[!] Error: Could not automatically locate wincmd.ini via Registry or AppData.")
		os.Exit(1)
	}

	fmt.Printf("[+] Total Commander Dir: %s\n", tcDir)
	fmt.Printf("[+] Found wincmd.ini:    %s\n", iniPath)

	plugins, err := config.ParsePluginsFromIni(iniPath, tcDir)
	if err != nil {
		fmt.Printf("[!] Failed to parse plugins: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[+] Extracting version information...")
	plugins = scanner.EnrichPluginVersions(plugins)

	categories := []struct {
		Type  domain.PluginType
		Title string
	}{
		{Type: domain.TypePacker, Title: "Packer / Archiver Plugins (WCX)"},
		{Type: domain.TypeLister, Title: "Lister / Viewer Plugins (WLX)"},
		{Type: domain.TypeFileSystem, Title: "File System Plugins (WFX)"},
		{Type: domain.TypeContent, Title: "Content / Meta Plugins (WDX)"},
	}

	var validCount, missingCount, uncertainCount int
	itemIndex := 1

	fmt.Printf("\nDiscovered %d installed plugins:\n", len(plugins))

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

			confidenceTag := fmt.Sprintf("[%s]", p.Confidence)
			if p.Confidence == domain.ConfidenceFallback || p.Confidence == domain.ConfidenceText {
				confidenceTag = fmt.Sprintf("⚠️ [%s]", p.Confidence)
				uncertainCount++
			}

			fmt.Printf("%2d. %-20s %-16s | Ver: %-12s %-15s -> %s\n",
				itemIndex, p.Name, statusSymbol, p.InstalledVersion, confidenceTag, p.Path)
			itemIndex++
		}
	}

	fmt.Println("\n================================================================================")
	fmt.Printf("SUMMARY: Total %d plugins | %d Valid ✓ | %d Missing ✗ | %d Uncertain ⚠️\n",
		len(plugins), validCount, missingCount, uncertainCount)

	if uncertainCount > 0 {
		fmt.Printf("[!] NOTICE:  %d plugin(s) rely on file dates or readme files. Version checks may require force update.\n", uncertainCount)
	}
	if missingCount > 0 {
		fmt.Printf("[!] WARNING: %d plugin file(s) were not found on disk!\n", missingCount)
	}
	fmt.Println("================================================================================")
}
