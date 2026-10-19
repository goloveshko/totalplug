package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/goloveshko/totalplug/internal/buildinfo"
	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
	"github.com/goloveshko/totalplug/internal/pipeline"
	"github.com/goloveshko/totalplug/internal/storage"
	"github.com/goloveshko/totalplug/internal/updater"
)

type cliOptions struct {
	forceRefresh bool
	iniPath      string
	update       bool
	updateAll    bool
	history      bool
	rollbackKey  string
	showVersion  bool
}

const usageLine = "Usage: totalplug [-r|--refresh-cache] [-c|--ini <path>] [-u|--update | --update-all | --history | --rollback <plugin>] [-v|--version]"

func parseArgs(args []string) (cliOptions, error) {
	var opts cliOptions
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--refresh-cache" || arg == "-r":
			opts.forceRefresh = true
		case arg == "--ini" || arg == "-c":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("flag %q requires a path argument", arg)
			}
			i++
			opts.iniPath = args[i]
		case strings.HasPrefix(arg, "--ini="):
			opts.iniPath = strings.TrimPrefix(arg, "--ini=")
		case arg == "--update" || arg == "-u":
			opts.update = true
		case arg == "--update-all":
			opts.updateAll = true
		case arg == "--history":
			opts.history = true
		case arg == "--rollback":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("flag %q requires a plugin name or id", arg)
			}
			i++
			opts.rollbackKey = args[i]
		case strings.HasPrefix(arg, "--rollback="):
			opts.rollbackKey = strings.TrimPrefix(arg, "--rollback=")
		case arg == "--version" || arg == "-v":
			opts.showVersion = true
		default:
			return opts, fmt.Errorf("unknown option %q", arg)
		}
	}
	if opts.history && opts.rollbackKey != "" {
		return opts, fmt.Errorf("--history and --rollback cannot be combined")
	}
	return opts, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// confirm reads a yes/no answer from the reader, defaulting to no on EOF.
func confirm(reader io.Reader, prompt string) bool {
	fmt.Print(prompt)
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// runUpdates applies every queued plugin update and reports per-plugin results.
// After each success the applied version is remembered in db (may be nil) so
// blind updates stop resurfacing as uncertain on the next scan.
func runUpdates(ctx context.Context, plugins []domain.Plugin, db *storage.Database) (succeeded int, failed int) {
	upd := updater.NewUpdater()
	// One scratch folder for the whole batch, taken away again on the way out:
	// a CLI run that finished should leave nothing in %TEMP%.
	defer config.ReleaseRunScratch()
	for _, p := range plugins {
		name := truncateString(p.Name, 18)
		if p.DownloadURL == "" {
			fmt.Printf("  [!] %-18s no download URL in catalog, update manually\n", name)
			failed++
			continue
		}
		fmt.Printf("  [⬆] %-18s %s -> %s (downloading...)\n", name, p.InstalledVersion, p.LatestVersion)
		outcome, err := upd.UpdatePlugin(ctx, p)
		if err != nil {
			fmt.Printf("  [✗] %-18s %v\n", name, err)
			if errors.Is(err, updater.ErrFileLocked) {
				fmt.Printf("      ↳ File is locked or read-only. Close Total Commander and run again.\n")
			}
			if errors.Is(err, updater.ErrUnsupportedArchive) {
				fmt.Printf("      ↳ Download manually and unpack the plugin yourself. Homepage: %s\n", p.WebURL)
			}
			failed++
			continue
		}
		fmt.Printf("  [✓] %-18s updated to %s (backup: %s)\n", name, p.LatestVersion, outcome.BackupPath)
		if db != nil {
			if err := db.RecordChange(storage.ChangeFromUpdate(p, outcome.BackupPath, outcome.AppliedFiles)); err != nil {
				fmt.Printf("      ↳ Could not remember version %s in history: %v\n", p.LatestVersion, err)
			}
		}
		succeeded++
	}
	return succeeded, failed
}

// readBlindAnswer prompts for one per-item decision: y = update this item,
// a = update this and all remaining items, anything else = skip.
func readBlindAnswer(in *bufio.Reader, prompt string) rune {
	fmt.Print(prompt)
	line, err := in.ReadString('\n')
	if err != nil {
		return 'n'
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return 'y'
	case "a", "all":
		return 'a'
	default:
		return 'n'
	}
}

// promptBlindUpdates lets the user decide item by item which updates with an
// unresolvable installed version should be applied anyway.
func promptBlindUpdates(reader io.Reader, candidates []domain.Plugin) []domain.Plugin {
	fmt.Printf("\nUnverified updates (%d): installed version could not be determined.\n", len(candidates))
	fmt.Println("Apply anyway? Answer per item: [y]es / [N]o / [a]ll remaining.")

	in := bufio.NewReader(reader)
	var selected []domain.Plugin
	allRemaining := false
	for i, p := range candidates {
		if allRemaining {
			selected = append(selected, p)
			continue
		}
		prompt := fmt.Sprintf("  %d. %-18s ? -> %s [y/N/a]: ", i+1, truncateString(p.Name, 18), p.LatestVersion)
		switch readBlindAnswer(in, prompt) {
		case 'y':
			selected = append(selected, p)
		case 'a':
			selected = append(selected, p)
			allRemaining = true
		}
	}
	return selected
}

func main() {
	// Parsed before the banner so `totalplug --version` and a mistyped flag both
	// answer with exactly one line.
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		fmt.Println(usageLine)
		os.Exit(1)
	}
	if opts.showVersion {
		fmt.Printf("totalplug %s\n", buildinfo.Version)
		return
	}

	fmt.Println("================================================================================")
	fmt.Println("                     TotalPlug - Total Commander Updater                        ")
	fmt.Println("================================================================================")

	db, dbErr := storage.NewDatabase("")
	if dbErr != nil {
		fmt.Printf("[!] Storage warning, some data was not loaded: %v\n", dbErr)
	}

	// The log answers both of these on its own: a rollback takes the folder and
	// the copy from the line it names, so neither waits on a scan of the install.
	if opts.history {
		printHistory(db)
		return
	}
	if opts.rollbackKey != "" {
		if err := runRollback(db, opts.rollbackKey, os.Stdin); err != nil {
			fmt.Printf("[!] %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Println("[+] Scanning Total Commander plugins...")
	res, err := pipeline.Scan(context.Background(), pipeline.Options{
		IniPath:      opts.iniPath,
		ForceRefresh: opts.forceRefresh,
	}, db)
	if err != nil {
		fmt.Printf("[!] Error: %v\n", err)
		os.Exit(1)
	}

	// The dir is empty when the ini came from %APPDATA% and the registry had no
	// InstallDir, i.e. when TotalPlug genuinely does not know where TC is installed.
	tcDir := res.TCDir
	if tcDir == "" {
		tcDir = "(unresolved; pass --ini <path to wincmd.ini> next to the install)"
	}
	fmt.Printf("[+] Total Commander Dir: %s\n", tcDir)
	fmt.Printf("[+] Found wincmd.ini:    %s\n", res.IniPath)

	plugins := res.Plugins
	if res.CatalogErr != nil {
		fmt.Printf("[!] Catalog fetch warning: %v\n", res.CatalogErr)
	} else if res.FromCache {
		fmt.Println("[i] Using cached catalog (revalidated with the CDN, or served offline). Use --refresh-cache to force a re-download.")
	} else {
		fmt.Println("[+] Fetched fresh catalog from GitHub CDN!")
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
	var updatable, unverified []domain.Plugin
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
			if p.Status == domain.StatusUncertain && p.DownloadURL != "" {
				unverified = append(unverified, p)
			}

			var statusSymbol string

			if p.Status == domain.StatusNotFound {
				statusSymbol = "✗ "
				missingCount++
			} else if p.Status == domain.StatusUpdateAvail {
				statusSymbol = "🚀"
				updateAvailCount++
				validCount++
				updatable = append(updatable, p)
			} else if p.Confidence == domain.ConfidenceFallback || p.Confidence == domain.ConfidenceText {
				statusSymbol = "⚠️"
				uncertainCount++
				validCount++
			} else {
				statusSymbol = "✓ "
				validCount++
			}

			confidenceTag := fmt.Sprintf("[%s]", p.Confidence)

			latestVerStr := ""
			if p.LatestVersion != "" && p.LatestVersion != "N/A (Not in catalog)" {
				latestVerStr = fmt.Sprintf("➔ Latest: %s", p.LatestVersion)
			}

			displayName := truncateString(p.Name, 18)

			fmt.Printf("%2d. %-18s %s | Ver: %-10s | %-14s %s\n",
				itemIndex, displayName, statusSymbol, p.InstalledVersion, confidenceTag, latestVerStr)
			itemIndex++
		}
	}

	fmt.Println("\n================================================================================")
	fmt.Printf("SUMMARY: Total %d | %d Valid ✓ | %d Updates 🚀 | %d Missing ✗ | %d Uncertain ⚠️\n",
		len(plugins), validCount, updateAvailCount, missingCount, uncertainCount)
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("LEGEND:")
	fmt.Println("  ✓  - Up to date")
	fmt.Println("  🚀 - Update available")
	fmt.Println("  ⚠️ - Uncertain version (check manually or force update)")
	fmt.Println("  ✗  - Plugin file missing on disk")

	if updateAvailCount > 0 {
		fmt.Printf("\n[🚀] SUCCESS: Found %d plugin update(s) available!\n", updateAvailCount)
	}
	if uncertainCount > 0 {
		fmt.Printf("[!] NOTICE:  %d plugin(s) rely on text files or manual overrides.\n", uncertainCount)
	}
	if missingCount > 0 {
		fmt.Printf("[!] WARNING: %d plugin file(s) were not found on disk!\n", missingCount)
	}
	fmt.Println("================================================================================")

	if len(updatable) == 0 && len(unverified) == 0 {
		return
	}
	if !opts.update && !opts.updateAll {
		fmt.Println("[i] Run with --update (interactive) or --update-all to apply available updates.")
		return
	}

	totalSucceeded, totalFailed := 0, 0

	if len(updatable) > 0 {
		fmt.Printf("\nPending updates (%d):\n", len(updatable))
		for i, p := range updatable {
			fmt.Printf("  %d. %-18s %s -> %s\n", i+1, truncateString(p.Name, 18), p.InstalledVersion, p.LatestVersion)
		}

		proceed := opts.updateAll
		if !proceed {
			prompt := "\n[?] Apply the updates listed above? [y/N]: "
			proceed = confirm(os.Stdin, prompt)
		}

		if proceed {
			fmt.Println()
			s, f := runUpdates(context.Background(), updatable, db)
			totalSucceeded += s
			totalFailed += f
		} else {
			fmt.Println("[i] Verified updates skipped.")
		}
	}

	if len(unverified) > 0 {
		if opts.updateAll {
			fmt.Printf("\n[i] Skipped %d unverified update(s): --update-all never applies unknown versions.\n", len(unverified))
		} else {
			selected := promptBlindUpdates(os.Stdin, unverified)
			if len(selected) > 0 {
				fmt.Println()
				s, f := runUpdates(context.Background(), selected, db)
				totalSucceeded += s
				totalFailed += f
			}
		}
	}

	if totalSucceeded+totalFailed == 0 {
		return
	}
	fmt.Printf("\nUPDATE SUMMARY: %d succeeded, %d failed\n", totalSucceeded, totalFailed)
	if totalFailed > 0 {
		os.Exit(1)
	}
}
