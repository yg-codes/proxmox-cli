package apt

import (
	"fmt"
	"strings"
)

// DisplayRepositories shows configured repo entries, diagnostics, and the
// standard handles PVE can add.
func DisplayRepositories(repos *Repositories) {
	fmt.Printf("\n📦 APT Repositories — Node: %s\n", repos.Node)
	fmt.Println(strings.Repeat("=", 130))
	fmt.Printf("Digest: %s\n", repos.Digest)

	// Configured repository entries
	fmt.Println("\nConfigured repositories:")
	fmt.Printf("%-50s %-4s %-11s %-26s %-16s %s\n",
		"FILE", "IDX", "STATUS", "COMPONENTS", "SUITES", "URI")
	fmt.Println(strings.Repeat("-", 130))

	count := 0
	for _, file := range repos.Files {
		for _, r := range file.Repos {
			status := "🔴 disabled"
			if r.Enabled {
				status = "🟢 enabled"
			}
			uri := ""
			if len(r.URIs) > 0 {
				uri = r.URIs[0]
			}
			fmt.Printf("%-50s %-4d %-11s %-26s %-16s %s\n",
				truncateStr(file.Path, 50), r.Index, status,
				truncateStr(strings.Join(r.Components, ","), 26),
				truncateStr(strings.Join(r.Suites, ","), 16),
				truncateStr(uri, 30))
			count++
		}
	}
	fmt.Println(strings.Repeat("-", 130))
	fmt.Printf("Total: %d entries in %d files\n", count, len(repos.Files))

	// Standard handles
	fmt.Println("\nStandard repositories (add or enable with 'repos add --handle <handle>'):")
	fmt.Printf("%-30s %-28s %s\n", "HANDLE", "NAME", "STATUS")
	fmt.Println(strings.Repeat("-", 76))
	for _, std := range repos.StandardRepos {
		status := "not configured"
		if std.Configured {
			if std.Enabled {
				status = "🟢 enabled"
			} else {
				status = "🔴 disabled"
			}
		}
		fmt.Printf("%-30s %-28s %s\n", std.Handle, truncateStr(std.Name, 28), status)
	}

	// Diagnostics: parse errors and warnings from the node
	hasDiag := len(repos.Errors) > 0
	for _, info := range repos.Infos {
		if info.Kind == "warning" {
			hasDiag = true
		}
	}
	if hasDiag {
		fmt.Println("\n⚠️  Diagnostics:")
		for _, e := range repos.Errors {
			fmt.Printf("   error:   %s\n", e)
		}
		for _, info := range repos.Infos {
			if info.Kind == "warning" {
				fmt.Printf("   warning: %s [%s] %s\n", info.Path, info.Property, info.Message)
			}
		}
	}

	fmt.Println(strings.Repeat("=", 130))
}

// DisplayRepository shows the entry a handle resolved to, before toggling it.
func DisplayRepository(r *Repository) {
	status := "disabled"
	if r.Enabled {
		status = "enabled"
	}
	fmt.Printf("  %s index %d — %s\n", r.FilePath, r.Index, status)
	fmt.Printf("    components: %s | suites: %s\n",
		strings.Join(r.Components, ", "), strings.Join(r.Suites, ", "))
	if len(r.URIs) > 0 {
		fmt.Printf("    uri: %s\n", r.URIs[0])
	}
}

func truncateStr(s string, max int) string {
	if max < 4 {
		return s
	}
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
