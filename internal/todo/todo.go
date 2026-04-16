package todo

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	ErrNoIncompleteItems = errors.New("no incomplete TODO items found")
	checkboxPattern      = regexp.MustCompile(`^(\s*(?:[-*+]|\d+\.)\s+\[)([ xX])(\]\s+)(.*)$`)
)

type Item struct {
	Line int
	Raw  string
	Text string
}

func (i Item) Signature() string {
	return fmt.Sprintf("%d:%s", i.Line, strings.TrimSpace(i.Raw))
}

func FindNextIncomplete(path string) (Item, error) {
	file, err := os.Open(path)
	if err != nil {
		return Item{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := scanner.Text()
		matches := checkboxPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		if strings.TrimSpace(strings.ToLower(matches[2])) != "" {
			continue
		}

		return Item{
			Line: lineNo,
			Raw:  line,
			Text: strings.TrimSpace(matches[4]),
		}, nil
	}

	if err := scanner.Err(); err != nil {
		return Item{}, err
	}

	return Item{}, ErrNoIncompleteItems
}
