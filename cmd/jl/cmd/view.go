package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

const (
	Red   = "\033[31m"
	Green = "\033[32m"
	Blue  = "\033[94m"
	Reset = "\033[0m"
)

var skip string
var focus string
var inputFile string
var raw bool
var showNoJson bool
var selector []string

func init() {
	viewCmd.Flags().StringVarP(&skip, "skip", "s", "", "comma separated list of keys to skip")
	viewCmd.Flags().StringVarP(&focus, "focus", "f", "", "comma separated list of keys to put into focus")
	viewCmd.Flags().StringVarP(&inputFile, "input", "i", "", "use file as input")
	viewCmd.Flags().BoolVarP(&raw, "raw", "r", false, "skip json keys")
	viewCmd.Flags().StringSliceVar(&selector, "select", []string{}, "filter logs by selector key=value")
	viewCmd.Flags().BoolVar(&showNoJson, "show-no-json", false, "also display log lines that are no json")
}

var viewCmd = &cobra.Command{
	Use:   "jl",
	Short: "jl: is a log display tool to display json based log streams or files beautifully",
	Example: `
# Show kubernetes log streams
kubectl logs my-pod -n my-namespace --follow | jl

# Show logs stored in a file
jl -i data/input.log

# Show logs stored in a file but skip the level and service properties
jl -i data/input.log -s level,service

# Show logs stored in a file but focus only on the message and reconcile_id property
jl -i data/input.log -f message,reconcile_id

# Show logs stored in a file but focus only on the message property, display only rows that match the select expressions
jl -i data/input.log -rf message,level --select=reconcile_id=fcdc26ae-7feb-4bf2-9058-f0c6666bc356 --select=level=info
`,
	Run: ViewLog,
}

func Execute() { // coverage-ignore
	cobra.CheckErr(viewCmd.Execute())
}

func ViewLog(_ *cobra.Command, _ []string) { // coverage-ignore
	scanner := bufio.NewScanner(os.Stdin)

	if len(inputFile) > 0 {
		file, err := os.Open(inputFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error opening file:", err)
			os.Exit(1)
		}
		defer file.Close()
		scanner = bufio.NewScanner(file)
	} else {
		scanner = bufio.NewScanner(os.Stdin)
	}

	for scanner.Scan() {
		txt := scanner.Text()
		var result interface{}
		err := json.Unmarshal([]byte(txt), &result)
		if err != nil {
			if showNoJson {
				// Print normal log
				fmt.Println(txt)
			}
			continue
		}

		data := result.(map[string]interface{})
		sortedKeys := generateKeys(data)
		printLine(sortedKeys, data)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "Error reading input:", err)
		os.Exit(1)
	}
}

func printLine(sortedKeys []string, data map[string]interface{}) {
	if len(selector) > 0 {
		for _, s := range selector {
			segments := strings.Split(s, "=")
			if len(segments) != 2 {
				continue
			}
			if data[segments[0]] != segments[1] {
				return
			}
		}
	}

	var line string
	for _, key := range sortedKeys {
		keyColor := Blue
		textColor := Reset
		if key == "message" || key == "msg" || key == "log" {
			textColor = Green
		}
		if key == "level" && data[key] == "error" {
			keyColor = Red
		}

		keyStr := fmt.Sprintf("%s%s%s", keyColor, key, Reset)
		valStr := fmt.Sprintf("%s%v%s", textColor, data[key], Reset)

		if raw {
			line += fmt.Sprintf("%s ", valStr)
		} else {
			line += fmt.Sprintf("%s: %s, ", keyStr, valStr)
		}
	}
	line = strings.Trim(line, ", ")
	fmt.Printf("%s\n", line)
}

func generateKeys(data map[string]interface{}) []string {
	sortedKeys := make([]string, 0, len(data))
	keys := make([]string, 0, len(data))
	toSkip := trim(strings.Split(skip, ","))
	toFocus := trim(strings.Split(focus, ","))

	if len(toFocus) == 0 {
		for key := range data {
			if !contains(toSkip, key) {
				keys = append(keys, key)
			}
		}

		sort.Strings(keys)

		sortedKeys = append(sortedKeys, keys...)
	} else {
		sortedKeys = toFocus
	}
	return sortedKeys
}

func trim(slice []string) []string {
	var result []string
	for _, s := range slice {
		if len(strings.TrimSpace(s)) > 0 {
			result = append(result, strings.TrimSpace(s))
		}
	}
	return result
}
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
