package main

import (
	"context"
	"fmt"
	"os"

	"totalplug/internal/checker"
	"totalplug/internal/config"
	"totalplug/internal/domain"
	"totalplug/internal/scanner"
	"totalplug/internal/storage"
)

func main() {
	fmt.Println("==========================================")
	fmt.Println("   TotalPlug - Total Commander Updater    ")
	fmt.Println("==========================================")

	// Check CLI flags
	forceRefresh := false
	for _, arg := range os.Args[1:] {
		if arg == "--refresh-cache" || arg == "-r" {
			forceRefresh = true
		}
	}

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

	db, err := storage.NewDatabase("")
	if err == nil {
		plugins = db.EnrichFromDatabase(plugins)
	}

	fmt.Println("[+] Checking updates against totalcmd.net...")
	chk := checker.NewChecker(forceRefresh)
	plugins, fromCache, err := chk.CheckUpdates(context.Background(), plugins)

	if err != nil {
		fmt.Printf("[!] Catalog fetch warning: %v\n", err)
	} else if fromCache {
		fmt.Println("[i] Used local 24h catalog cache. (Use --refresh-cache to force update)")
	} else {
		fmt.Println("[+] Fetched fresh catalog from totalcmd.net!")
	}

	categories := []struct {
		Type  domain.PluginType
		Title string
	}{
		{Type: domain.TypePacker, Title: "Packer / Archiver Plugins (WCX)"},
		{Type: domain.TypeLister, Title: "Lister / Viewer Plugins (WLX)"},
		{Type: domain.TypeFileSystem, Title: "File System Plugins (WFX)"},
		{Type: domain.TypeContent, Title: "Content / Meta Plugins (WDX)"},
	}

	var validCount, missingCount, updateAvailCount, uncertainCount int
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
				statusSymbol = "✗ [Missing]"
				missingCount++
			} else if p.Status == domain.StatusUpdateAvail {
				statusSymbol = "🚀 [UPDATE AVAIL]"
				updateAvailCount++
				validCount++
			} else {
				validCount++
			}

			confidenceTag := fmt.Sprintf("[%s]", p.Confidence)
			if p.Confidence == domain.ConfidenceFallback || p.Confidence == domain.ConfidenceText {
				confidenceTag = fmt.Sprintf("⚠️ [%s]", p.Confidence)
				uncertainCount++
			}

			latestVerStr := ""
			if p.LatestVersion != "" && p.LatestVersion != "N/A (Not in catalog)" {
				latestVerStr = fmt.Sprintf(" ➔ Latest: %s", p.LatestVersion)
			}

			fmt.Printf("%2d. %-20s %-18s | Ver: %-10s%-12s%s\n",
				itemIndex, p.Name, statusSymbol, p.InstalledVersion, confidenceTag, latestVerStr)
			itemIndex++
		}
	}

	fmt.Println("\n================================================================================")
	fmt.Printf("SUMMARY: Total %d | %d Valid ✓ | %d Updates 🚀 | %d Missing ✗ | %d Uncertain ⚠️\n",
		len(plugins), validCount, updateAvailCount, missingCount, uncertainCount)

	if updateAvailCount > 0 {
		fmt.Printf("[🚀] SUCCESS: Found %d plugin update(s) available on totalcmd.net!\n", updateAvailCount)
	}
	if uncertainCount > 0 {
		fmt.Printf("[!] NOTICE:  %d plugin(s) rely on file dates or readme files.\n", uncertainCount)
	}
	if missingCount > 0 {
		fmt.Printf("[!] WARNING: %d plugin file(s) were not found on disk!\n", missingCount)
	}
	fmt.Println("================================================================================")
}
