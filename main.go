// go-latest tries to upgrade programs go install-d to GOBIN.
//
// For reference, go itself:
// ./src/cmd/go/internal/version/version.go
// ./src/cmd/go/internal/work/build.go
package main

import (
	"context"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"

	"golang.org/x/mod/semver"
)

// NOTES:
// golang.org/x/mod/semver
// golang.org/x/mod/module
//
// Use:
// go list -m -json golang.org/x/tools/gopls@latest
// either on each pkg or on the module.

func gobin() string {
	gobin := os.Getenv("GOBIN")
	if gobin != "" {
		return gobin
	}
	home := os.Getenv("HOME")
	if home != "" {
		return filepath.Join(home, "go", "bin")
	}
	return ""
}

func listPrograms(dir string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var programs []string
	for _, fi := range files {
		if isExecutable(fi) {
			programs = append(programs, filepath.Join(dir, fi.Name()))
		}
	}
	return programs, nil
}

func isExecutable(fi fs.FileInfo) bool {
	return fi.Mode().Perm()&0111 != 0
}

// isSpecific revision installed like from local repo or a specific SHA.
func isSpecific(v string) bool {
	// Local
	if v == "(devel)" {
		return true
	}
	// Specific SHA or otherwise not a "clean" version.
	if semver.IsValid(v) && semver.Prerelease(v) != "" {
		return true
	}
	return false
}

func latest(ctx context.Context, pkg string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-json", pkg+"@latest")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go list (%s):\n%s", err, out)
	}
	listing := struct {
		Version string
	}{}
	err = json.Unmarshal(out, &listing)
	if err != nil {
		return "", fmt.Errorf("json unmarshal: %v", err)
	}
	return listing.Version, nil
}

func installer(ctx context.Context) error {
	dir := gobin()
	if dir == "" {
		return errors.New("GOBIN not found")
	}
	progs, err := listPrograms(dir)
	if err != nil {
		return err
	}

	for _, f := range progs {
		info, err := buildinfo.ReadFile(f)
		if err != nil {
			return err
		}
		if isSpecific(info.Main.Version) {
			fmt.Printf("%s %s skip\n", info.Path, info.Main.Version)
			continue
		}
		// Latest available is checked per module.
		// TODO: Cache this lookup.
		target, err := latest(ctx, info.Main.Path)
		if err != nil {
			fmt.Printf("%s\n", err)
			// TODO: Doesn't work for golang.org/x/tools/cmd/auth/authtest
			target = "?"
		}
		if info.Main.Version == target {
			// fmt.Printf("%s %s already latest\n", info.Path, info.Main.Version)
			continue
		}
		fmt.Printf("%s %s -> %s\n", info.Path, info.Main.Version, target)
		// TODO: Is it faster to combine packages from the same module into a single exec?
		cmd := exec.CommandContext(ctx, "go", "install", info.Path+"@latest")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("go install (%s):\n%s", err, out)
		}
		// TODO: If no longer present in module or deprecated, ask if remove?
	}

	return nil
}

func runMain() error {
	showVersion := flag.Bool("v", false, "print version")
	flag.Parse()

	if *showVersion {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return errors.New("could not read buildinfo")
		}
		fmt.Println(bi.Main.Version)
		return nil
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	err := installer(ctx)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	err := runMain()
	if err != nil {
		fmt.Printf("%s", err.Error())
		os.Exit(1)
	}
}
