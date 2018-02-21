package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	lunchyVersion = "0.2.1"
)

var (
	launchAgentsPath = filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents")
)

func pPath(n string) string {
	return filepath.Join(launchAgentsPath, n+".plist")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileCopy(src string, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}

	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}

	return d.Close()
}

func findPlists(path string) []string {
	output, err := exec.Command("find", "-L", path, "-name", "*.plist", "-type", "f").Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	plists := make([]string, 0, len(lines))

	for _, line := range lines {
		plists = append(plists, strings.Replace(filepath.Base(line), ".plist", "", 1))
	}

	sort.Sort(sort.StringSlice(plists))

	return plists
}

func getPlists() []string {
	return findPlists(launchAgentsPath)
}

func sliceIncludes(slice []string, match string) bool {
	for _, val := range slice {
		if val == match {
			return true
		}
	}

	return false
}

func printUsage(_ []string) error {
	fmt.Printf("Lunchy %s, the friendly launchctl wrapper\n", lunchyVersion)
	fmt.Println("Usage: lunchy [start|stop|restart|list|status|install|show|edit|remove|scan] [options]")
	return nil
}

func printList(_ []string) error {
	for _, file := range getPlists() {
		fmt.Println(file)
	}
	return nil
}

func printStatus(args []string) error {
	out, err := exec.Command("launchctl", "list").Output()

	if err != nil {
		return fmt.Errorf("failed to get process list: %s", err)
	}

	pattern := ""

	if len(args) == 3 {
		pattern = args[2]
	}

	installed := getPlists()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")

	for _, line := range lines {
		chunks := strings.Split(line, "\t")

		// Only show services from the user's LaunchAgents.
		if !sliceIncludes(installed, chunks[2]) {
			continue
		}

		// Filter on service name, if given.
		if len(pattern) > 0 && !strings.Contains(chunks[2], pattern) {
			continue
		}

		// Replace tabs with spaces to condense output.
		fmt.Println(strings.Replace(line, "\t", " ", -1))
	}

	return nil
}

func assertValidArgs(args []string, msg string) {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(1)
	}
}

func withProfile(f func(string) error) func(args []string) error {
	return func(args []string) error {
		// Check if name pattern is not given and try profiles
		if len(args) == 2 {
			p, err := readProfile()
			if err != nil {
				return err
			}
			if p == nil {
				return fmt.Errorf("name required")
			}
			return plistsAction(p, f)
		}

		name := args[2]

		for _, plist := range getPlists() {
			if strings.Contains(plist, name) {
				f(plist)
			}
		}

		return nil
	}
}

func runLaunchCtl(verb string, name string) error {
	path := pPath(name)
	_, err := exec.Command("launchctl", verb, path).Output()

	if err != nil {
		return fmt.Errorf("failed to %s %s: %s", verb, name, err)
	}

	fmt.Println(verb, name)

	return nil
}

func startDaemon(name string) error {
	return runLaunchCtl("load", name)
}

func stopDaemon(name string) error {
	return runLaunchCtl("unload", name)
}

func stopStartDaemon(name string) error {
	stopDaemon(name) // Ignore errors on stop.
	return startDaemon(name)
}

func withFirstMatch(f func(string) error) func([]string) error {
	return func(args []string) error {
		assertValidArgs(args, "name required")
		name := args[2]

		for _, plist := range getPlists() {
			if strings.Contains(plist, name) {
				return f(plist)
			}
		}

		return fmt.Errorf("not found: %s", name)
	}
}

func showPlist(name string) error {
	path := pPath(name)
	contents, err := ioutil.ReadFile(path)

	if err != nil {
		return fmt.Errorf("unable to read plist: %s", err)
	}

	fmt.Printf(string(contents))

	return nil
}

func editPlist(name string) error {
	path := pPath(name)
	editor := os.Getenv("EDITOR")

	if len(editor) == 0 {
		return fmt.Errorf("EDITOR environment variable is not set")
	}

	cmd := exec.Command(editor, path)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	cmd.Start()
	cmd.Wait()

	return nil
}

func installPlist(args []string) error {
	assertValidArgs(args, "path required")

	path := args[2]

	if !fileExists(path) {
		return fmt.Errorf("source file \"%s\" does not exist", path)
	}

	info, _ := os.Stat(path)
	newPath := filepath.Join(launchAgentsPath, info.Name())

	if fileExists(newPath) && os.Remove(newPath) != nil {
		return fmt.Errorf("unable to delete existing plist")
	}

	if fileCopy(path, newPath) != nil {
		return fmt.Errorf("failed to copy file")
	}

	fmt.Println(path, "installed to", launchAgentsPath)
	return nil
}

func removePlist(args []string) error {
	assertValidArgs(args, "name required")

	name := args[2]

	for _, plist := range getPlists() {
		if strings.Contains(plist, name) {
			path := pPath(plist)

			if os.Remove(path) == nil {
				fmt.Println("removed", path)
			} else {
				fmt.Println("failed to remove", path)
			}
		}
	}

	return nil
}

func scanPath(args []string) error {
	path := launchAgentsPath

	if len(args) >= 3 {
		path = args[2]
	}

	// This is a handy override to find all homebrew-based lists
	if path == "homebrew" {
		path = "/usr/local/Cellar"
	}

	for _, f := range findPlists(path) {
		fmt.Println(f)
	}

	return nil
}

// Get daemon names specified in lunchy profile
func readProfile() ([]string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, ".lunchy")

	if !fileExists(path) {
		return nil, nil
	}

	fmt.Println("Using daemons in profile:", path)
	buff, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(buff)), "\n")
	result := make([]string, 0, len(lines))

	for _, l := range lines {
		line := strings.TrimSpace(l)

		// Skip comments (starts with #)
		if line[0] == '#' {
			continue
		}

		result = append(result, line)
	}

	return result, nil
}

func plistsAction(names []string, f func(string) error) error {
	plists := getPlists()

	for _, name := range names {
		for _, plist := range plists {
			if strings.Contains(plist, name) {
				if err := f(plist); err != nil {
					fmt.Println(err)
				}
			}
		}
	}

	return nil
}

func main() {
	if len(os.Args) == 1 {
		printUsage(os.Args)
		os.Exit(1)
	}

	f, ok := map[string](func([]string) error){
		"add":     installPlist,
		"edit":    withFirstMatch(editPlist),
		"help":    printUsage,
		"install": installPlist,
		"list":    printList,
		"ls":      printList,
		"ps":      printStatus,
		"remove":  removePlist,
		"restart": withProfile(stopStartDaemon),
		"rm":      removePlist,
		"scan":    scanPath,
		"show":    withFirstMatch(showPlist),
		"start":   withProfile(startDaemon),
		"status":  printStatus,
		"stop":    withProfile(stopDaemon),
	}[os.Args[1]]

	if !ok {
		printUsage(os.Args)
		os.Exit(1)
	}

	err := f(os.Args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
