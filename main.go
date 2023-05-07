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
	"runtime"
	"runtime/debug"

	"golang.org/x/mod/semver"
	"golang.org/x/sync/errgroup"
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

// isSpecific revision installed from local repo or a specific SHA.
// In other words not some generally available package installed with @latest.
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

// latest version of package, or error.
func latest(ctx context.Context, pkg string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-json", pkg+"@latest")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go list (%w):\n%s", err, out)
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

func installer(ctx context.Context, nProcs int, latestGo bool) error {
	dir := gobin()
	if dir == "" {
		return errors.New("GOBIN not found")
	}
	progs, err := listPrograms(dir)
	if err != nil {
		return err
	}

	var eg errgroup.Group
	eg.SetLimit(nProcs)

	for _, f := range progs {
		ff := f
		eg.Go(func() error {
			info, err := buildinfo.ReadFile(ff)
			if err != nil {
				return err
			}
			if isSpecific(info.Main.Version) {
				fmt.Printf("%s %s skip\n", info.Path, info.Main.Version)
				return nil
			}

			// Latest available is checked per module.
			// TODO: Cache this lookup.
			target, err := latest(ctx, info.Main.Path)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				fmt.Printf("%s\n", err)
				// TODO: Doesn't work for golang.org/x/tools/cmd/auth/authtest
				target = "?"
			}

			goUpgrade := latestGo && runtime.Version() != info.GoVersion
			modUpgrade := target != info.Main.Version
			if !goUpgrade && !modUpgrade {
				fmt.Printf("%s %s already latest\n", info.Path, info.Main.Version)
				return nil
			}
			fmt.Printf("%s %s -> %s\n", info.Path, info.Main.Version, target)

			// TODO: Is it faster to combine packages from the same module into a single exec?
			cmd := exec.CommandContext(ctx, "go", "install", info.Path+"@latest")
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("go install (%s):\n%s", err, out)
			}
			// TODO: If no longer present in module or deprecated, ask if remove?
			return nil
		})

	}

	return eg.Wait()
}

const help = `Usage: go-latest [options]

Install the latest version of go install'd programs in GOBIN.

Options:
`

func runMain() error {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), help)
		flag.PrintDefaults()
	}
	showVersion := flag.Bool("v", false, "Print version and exit")
	nProcs := flag.Int("j", 0, "Number of parallel workers, defaults to number of CPUs")
	latestGo := flag.Bool("go", false, "Re-install programs not built with the current version of Go")
	flag.Parse()

	if *showVersion {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return errors.New("could not read buildinfo")
		}
		fmt.Println(bi.Main.Version)
		return nil
	}
	if *nProcs == 0 {
		*nProcs = runtime.NumCPU()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("make temp dir: %w", err)
	}
	defer os.Remove(dir)
	err = os.Chdir(dir)
	if err != nil {
		return fmt.Errorf("chdir: %w", err)
	}

	err = installer(ctx, *nProcs, *latestGo)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	err := runMain()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			fmt.Printf("%s", err.Error())
		}
		os.Exit(1)
	}
}
