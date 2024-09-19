package jira

import (
	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/timeutil"
	gitea_context "code.gitea.io/gitea/services/context"
	"xorm.io/builder"
)

type JiraIssueRelatedCommit struct {
	ID          int64              `xorm:"pk autoincr"`
	RepoID      int64              `xorm:"not null index(jira_issue_ticket) index(jira_issue_repo)"`
	Ticket      string             `xorm:"not null index(jira_issue_ticket)"`
	SHA         string             `xorm:"varchar(64) not null"`
	CreatedUnix timeutil.TimeStamp `xorm:"not null index(jira_issue_ticket)"`
}

type JiraIssueRelatedCommitKey struct {
	SHA    string
	Ticket string
}

type FindJiraIssueRelatedCommitOptions struct {
	db.ListOptions
	RepoID int64
}

type FindJiraIssueRelatedCommitByTicketOptions struct {
	db.ListOptions
	Ticket string
	Doer   *user.User
}

func (opts FindJiraIssueRelatedCommitOptions) ToConds() builder.Cond {
	var cond builder.Cond = builder.Eq{"repo_id": opts.RepoID}
	return cond
}

func (opts FindJiraIssueRelatedCommitOptions) ToOrders() string {
	return "created_unix DESC, id DESC"
}

func (opts FindJiraIssueRelatedCommitByTicketOptions) ToConds() builder.Cond {
	cond := builder.Eq{"ticket": opts.Ticket}
	return cond.And(builder.In("repo_id", opts.accessibleRepos()))
}

func (opts FindJiraIssueRelatedCommitByTicketOptions) ToOrders() string {
	return "repo_id, created_unix asc"
}

func (opts FindJiraIssueRelatedCommitByTicketOptions) accessibleRepos() *builder.Builder {
	searchOpts := repo.SearchRepoOptions{
		Actor:      opts.Doer,
		OwnerID:    opts.Doer.ID,
		Private:    true,
		AllPublic:  true,
		AllLimited: true,
	}

	return builder.Select("id").From("repository").Where(repo.SearchRepositoryCondition(&searchOpts))
}

type JiraIssue struct {
	TicketId     string
	TotalCommits int
	Commits      []*JiraIssueRelatedCommit
}

func GetJiraIssueByTicket(ctx *gitea_context.Context, criterion FindJiraIssueRelatedCommitByTicketOptions) (*JiraIssue, error) {
	commits := make([]*JiraIssueRelatedCommit, 0, 10)

	count, err := db.GetEngine(ctx).Where(criterion.ToConds()).Count(new(JiraIssueRelatedCommit))

	if err != nil {
		log.Error("Error while trying to count JiraIssueRelatedCommit: %v", err)
		return nil, err
	}

	if count == 0 {
		return &JiraIssue{TicketId: criterion.Ticket, Commits: commits, TotalCommits: int(count)}, nil
	}

	offset, limit := criterion.GetSkipTake()

	if err = db.GetEngine(ctx).Where(criterion.ToConds()).OrderBy(criterion.ToOrders()).Limit(limit, offset).Find(&commits); err != nil {
		log.Error("Error while trying to fetch JiraIssueRelatedCommit: %v", err)
		return nil, err
	}

	return &JiraIssue{TicketId: criterion.Ticket, Commits: commits, TotalCommits: int(count)}, nil
}

func (issues *JiraIssue) CommitsByRepo() map[int64][]*JiraIssueRelatedCommit {
	result := make(map[int64][]*JiraIssueRelatedCommit)

	for _, commit := range issues.Commits {
		if _, ok := result[commit.RepoID]; !ok {
			result[commit.RepoID] = []*JiraIssueRelatedCommit{}
		}
		result[commit.RepoID] = append(result[commit.RepoID], commit)
	}

	return result
}

func (commit JiraIssueRelatedCommit) GetKey() JiraIssueRelatedCommitKey {
	return JiraIssueRelatedCommitKey{
		Ticket: commit.Ticket,
		SHA:    commit.SHA,
	}
}
