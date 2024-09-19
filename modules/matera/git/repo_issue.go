package git

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	jira_issue_models "code.gitea.io/gitea/models/matera/jira"
	git_module "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"
)

var issueRegex = regexp.MustCompile("^([A-Z]*-\\d+)")

type Parser struct {
	scanner *bufio.Scanner
}

func GetIssueRelateCommits(repo *git_module.Repository, page int, pageSize int) ([]*jira_issue_models.JiraIssueRelatedCommit, int, error) {
	stdoutReader, stdoutWriter := io.Pipe()
	defer stdoutReader.Close()
	defer stdoutWriter.Close()
	stderr := strings.Builder{}

	rc := &git_module.RunOpts{Dir: repo.Path, Stdout: stdoutWriter, Stderr: &stderr}

	go func() {
		err := git_module.NewCommand("log").
			AddOptionFormat("--pretty=format:%s", "%H %at %s").
			AddArguments("--all").Run(repo.Ctx, rc)
		if err != nil {
			_ = stdoutWriter.CloseWithError(git_module.ConcatenateError(err, stderr.String()))
		} else {
			_ = stdoutWriter.Close()
		}
	}()

	parser := NewParser(stdoutReader)
	commits, err := parser.ParseCommits()

	if err != nil {
		return nil, 0, err
	}

	//sortTagsByTime(tags)
	commitsTotal := len(commits)
	if page != 0 {
		commits = util.PaginateSlice(commits, page, pageSize).([]*jira_issue_models.JiraIssueRelatedCommit)
	}

	return commits, commitsTotal, nil
}

func NewParser(r io.Reader) *Parser {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	refDelim := make([]byte, 0, 1)
	refDelim = append(refDelim, '\n')

	// Split input into delimiter-separated "reference blocks".
	scanner.Split(
		func(data []byte, atEOF bool) (advance int, token []byte, err error) {
			// Scan until delimiter, marking end of reference.
			delimIdx := bytes.Index(data, refDelim)
			if delimIdx >= 0 {
				token := data[:delimIdx]
				advance := delimIdx + len(refDelim)
				return advance, token, nil
			}
			// If we're at EOF, we have a final, non-terminated reference. Return it.
			if atEOF {
				return len(data), data, nil
			}
			// Not yet a full field. Request more data.
			return 0, nil, nil
		})

	return &Parser{
		scanner: scanner,
	}
}

func (parser *Parser) ParseCommits() ([]*jira_issue_models.JiraIssueRelatedCommit, error) {
	var commits []*jira_issue_models.JiraIssueRelatedCommit
	for {
		commit, hasNext, err := parser.next()
		if err != nil {
			return nil, fmt.Errorf("GetIssueRelateCommits: parse commit: %w", err)
		} else if commit != nil {
			commits = append(commits, commit)
		}
		if !hasNext {
			return commits, nil
		}
	}
}

func (parser *Parser) next() (*jira_issue_models.JiraIssueRelatedCommit, bool, error) {
	if !parser.scanner.Scan() {
		return nil, false, nil
	}
	line := parser.scanner.Text()
	if line == "" {
		return nil, false, nil
	} else {
		parts := strings.SplitN(line, " ", 3)
		ticket := issueRegex.FindString(parts[2])
		if ticket == "" {
			return nil, true, nil
		} else {
			timestamp, err := parser.toTimestamp(parts[1])
			if err != nil {
				return nil, false, err
			}

			return &jira_issue_models.JiraIssueRelatedCommit{
				Ticket:      ticket,
				SHA:         parts[0],
				CreatedUnix: timestamp,
			}, true, nil
		}
	}
}

func (parser *Parser) toTimestamp(value string) (timeutil.TimeStamp, error) {
	asInt, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return timeutil.TimeStampNow(), err
	} else {
		return timeutil.TimeStamp(asInt), nil
	}
}
