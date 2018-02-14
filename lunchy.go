package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
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
	plists := []string{}

	for _, line := range lines {
		plists = append(plists, strings.Replace(filepath.Base(line), ".plist", "", 1))
	}

	return plists
}

func getPlists() []string {
	return findPlists(launchAgentsPath)
}

func getPlist(name string) string {
	for _, plist := range getPlists() {
		if strings.Index(plist, name) != -1 {
			return plist
		}
	}

	return ""
}

func sliceIncludes(slice []string, match string) bool {
	for _, val := range slice {
		if val == match {
			return true
		}
	}

	return false
}

func printUsage() {
	fmt.Printf("Lunchy %s, the friendly launchctl wrapper\n", lunchyVersion)
	fmt.Println("Usage: lunchy [start|stop|restart|list|status|install|show|edit|remove|scan] [options]")
}

func printList() {
	for _, file := range getPlists() {
		fmt.Println(file)
	}
}

func printStatus(args []string) {
	out, err := exec.Command("launchctl", "list").Output()

	if err != nil {
		fatal("failed to get process list")
	}

	pattern := ""

	if len(args) == 3 {
		pattern = args[2]
	}

	installed := getPlists()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")

	for _, line := range lines {
		chunks := strings.Split(line, "\t")
		cleanLine := strings.Replace(line, "\t", " ", -1)

		if len(pattern) > 0 {
			if strings.Index(chunks[2], pattern) != -1 {
				if sliceIncludes(installed, chunks[2]) {
					fmt.Println(cleanLine)
				}
			}
		} else {
			if sliceIncludes(installed, chunks[2]) {
				fmt.Println(cleanLine)
			}
		}
	}
}

func exitWithInvalidArgs(args []string, msg string) {
	if len(args) < 3 {
		fmt.Println(msg)
		os.Exit(1)
	}
}

func startDaemons(args []string) {
	// Check if name pattern is not given and try profiles
	if len(args) == 2 {
		if profileExists() {
			startProfile()
			return
		}
		exitWithInvalidArgs(args, "name required")
	}

	name := args[2]

	for _, plist := range getPlists() {
		if strings.Index(plist, name) != -1 {
			startDaemon(plist)
		}
	}
}

func startDaemon(name string) {
	path := pPath(name)
	_, err := exec.Command("launchctl", "load", path).Output()

	if err != nil {
		fmt.Println("failed to start", name)
		return
	}

	fmt.Println("started", name)
}

func stopDaemons(args []string) {
	// Check if name pattern is not given and try profiles
	if len(args) == 2 {
		if profileExists() {
			stopProfile()
			return
		}
		exitWithInvalidArgs(args, "name required")
	}

	name := args[2]

	for _, plist := range getPlists() {
		if strings.Index(plist, name) != -1 {
			stopDaemon(plist)
		}
	}
}

func stopDaemon(name string) {
	path := pPath(name)
	_, err := exec.Command("launchctl", "unload", path).Output()

	if err != nil {
		fmt.Println("failed to stop", name)
		return
	}

	fmt.Println("stopped", name)
}

func restartDaemons(args []string) {
	// Check if name pattern is not given and try profiles
	if len(args) == 2 {
		if profileExists() {
			restartProfile()
			return
		}
		exitWithInvalidArgs(args, "name required")
	}

	name := args[2]

	for _, plist := range getPlists() {
		if strings.Index(plist, name) != -1 {
			stopDaemon(plist)
			startDaemon(plist)
		}
	}
}

func showPlist(args []string) {
	exitWithInvalidArgs(args, "name required")

	name := args[2]

	for _, plist := range getPlists() {
		if strings.Index(plist, name) != -1 {
			printPlistContent(plist)
			return
		}
	}
}

func printPlistContent(name string) {
	path := pPath(name)
	contents, err := ioutil.ReadFile(path)

	if err != nil {
		fatal("unable to read plist")
	}

	fmt.Printf(string(contents))
}

func editPlist(args []string) {
	exitWithInvalidArgs(args, "name required")

	name := args[2]

	for _, plist := range getPlists() {
		if strings.Index(plist, name) != -1 {
			editPlistContent(plist)
			return
		}
	}
}

func editPlistContent(name string) {
	path := pPath(name)
	editor := os.Getenv("EDITOR")

	if len(editor) == 0 {
		fatal("EDITOR environment variable is not set")
	}

	cmd := exec.Command(editor, path)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	cmd.Start()
	cmd.Wait()
}

func installPlist(args []string) {
	exitWithInvalidArgs(args, "path required")

	path := args[2]

	if !fileExists(path) {
		fatal("source file does not exist")
	}

	info, _ := os.Stat(path)
	newPath := filepath.Join(launchAgentsPath, info.Name())

	if fileExists(newPath) && os.Remove(newPath) != nil {
		fatal("unable to delete existing plist")
	}

	if fileCopy(path, newPath) != nil {
		fatal("failed to copy file")
	}

	fmt.Println(path, "installed to", launchAgentsPath)
}

func removePlist(args []string) {
	exitWithInvalidArgs(args, "name required")

	name := args[2]

	for _, plist := range getPlists() {
		if strings.Index(plist, name) != -1 {
			path := pPath(plist)

			if os.Remove(path) == nil {
				fmt.Println("removed", path)
			} else {
				fmt.Println("failed to remove", path)
			}
		}
	}
}

func scanPath(args []string) {
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
}

// Get full path to lunchy profile file
func profilePath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, ".lunchy")
}

// Check if profile file exists
func profileExists() bool {
	return fileExists(profilePath())
}

// Get daemon names specified in lunchy profile
func readProfile() []string {
	path := profilePath()
	if path == "" {
		return nil
	}

	buff, err := ioutil.ReadFile(path)
	if err != nil {
		return nil
	}

	result := []string{}
	lines := strings.Split(strings.TrimSpace(string(buff)), "\n")

	for _, l := range lines {
		line := strings.TrimSpace(l)

		// Skip comments (starts with #)
		if line[0] == '#' {
			continue
		}

		result = append(result, line)
	}

	return result
}

func plistsAction(names []string, action string) {
	plists := getPlists()

	for _, name := range names {
		for _, plist := range plists {
			if strings.Index(plist, name) != -1 {
				switch action {
				case "start":
					startDaemon(plist)
				case "stop":
					stopDaemon(plist)
				case "restart":
					stopDaemon(plist)
					startDaemon(plist)
				}
			}
		}
	}
}

func startProfile() {
	fmt.Println("Starting daemons in profile:", profilePath())
	plistsAction(readProfile(), "start")
}

func stopProfile() {
	fmt.Println("Stopping daemons in profile:", profilePath())
	plistsAction(readProfile(), "stop")
}

func restartProfile() {
	fmt.Println("Restarting daemons in profile:", profilePath())
	plistsAction(readProfile(), "restart")
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

func wrap(f func()) func([]string) {
	return func(_ []string) {
		f()
	}
}

func main() {
	if len(os.Args) == 1 {
		printUsage()
		os.Exit(1)
	}

	m := map[string](func([]string)){
		"add":     installPlist,
		"edit":    editPlist,
		"help":    wrap(printUsage),
		"install": installPlist,
		"list":    wrap(printList),
		"ls":      wrap(printList),
		"ps":      printStatus,
		"remove":  removePlist,
		"restart": restartDaemons,
		"rm":      removePlist,
		"scan":    scanPath,
		"show":    showPlist,
		"start":   startDaemons,
		"status":  printStatus,
		"stop":    stopDaemons,
	}
	if f, ok := m[os.Args[1]]; ok {
		f(os.Args)
	} else {
		printUsage()
		os.Exit(1)
	}
}
