package jira

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"code.gitea.io/gitea/models/db"
	git_model "code.gitea.io/gitea/models/git"
	issue_model "code.gitea.io/gitea/models/matera/jira"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/gitrepo"
	git_repo_module "code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/graceful"
	"code.gitea.io/gitea/modules/log"
	matera_repo_module "code.gitea.io/gitea/modules/matera/repository"
	"code.gitea.io/gitea/modules/queue"
	gitea_context "code.gitea.io/gitea/services/context"
	"xorm.io/builder"
)

type JiraIssueInfo struct {
	TicketId     string
	Repositories []*RepositoryInfo
	TotalCommits int
}

type RepositoryInfo struct {
	Repository *repo_model.Repository
	PullMirror *repo_model.Mirror
	Commits    []*git_model.SignCommitWithStatuses
}

type SyncJiraIssueOptions struct {
	RepoID int64
}

// branchSyncQueue represents a queue to handle branch sync jobs.
var syncJiraIssuesQueue *queue.WorkerPoolQueue[*SyncJiraIssueOptions]

func Init(ctx context.Context) error {
	return initSyncJiraIssuesQueue(graceful.GetManager().ShutdownContext())
}

func (issue *JiraIssueInfo) IsEmpty() bool {
	return issue.Repositories == nil || len(issue.Repositories) == 0
}

func AddAllReposToSyncJiraIssuesQueue(ctx context.Context, doerID int64) error {
	if err := db.Iterate(ctx, builder.Eq{"is_empty": false}, func(ctx context.Context, repo *repo_model.Repository) error {
		return AddRepoToSyncJiraIssuesQueue(repo.ID, doerID)
	}); err != nil {
		return fmt.Errorf("run sync all branches failed: %v", err)
	}
	return nil
}

func AddRepoToSyncJiraIssuesQueue(repoID, doerID int64) error {
	return syncJiraIssuesQueue.Push(&SyncJiraIssueOptions{
		RepoID: repoID,
	})
}

func FindJiraIssue(ctx *gitea_context.Context, page int, pageSize int) (*JiraIssueInfo, error) {
	ticketId := ctx.PathParam("ticketId")

	issue, err := issue_model.GetJiraIssueByTicket(ctx, issue_model.FindJiraIssueRelatedCommitByTicketOptions{
		ListOptions: db.ListOptions{
			Page:     page,
			PageSize: pageSize,
		},
		Ticket: ticketId,
		Doer:   ctx.Doer,
	})

	if err != nil {
		return nil, err
	} else {
		return createJiraIssueInfo(ctx, issue), nil
	}
}

func createJiraIssueInfo(ctx *gitea_context.Context, issue *issue_model.JiraIssue) *JiraIssueInfo {
	repositories := make([]*RepositoryInfo, 0, 10)
	commitsByRepo := issue.CommitsByRepo()

	for repoId, commits := range commitsByRepo {
		repoInfo, _ := createRepositoryInfo(ctx, repoId, commits)
		if repoInfo != nil {
			repositories = append(repositories, repoInfo)

			sort.Slice(repositories, func(i, j int) bool {
				return repositories[i].Repository.ID < repositories[j].Repository.ID
			})
		}

	}

	return &JiraIssueInfo{
		TicketId:     issue.TicketId,
		Repositories: repositories,
		TotalCommits: issue.TotalCommits,
	}
}

func createRepositoryInfo(ctx *gitea_context.Context, repoId int64, commits []*issue_model.JiraIssueRelatedCommit) (*RepositoryInfo, error) {
	repoInfo := RepositoryInfo{}

	if err := repoInfo.loadRepository(ctx, repoId); err != nil {
		log.Warn("createRepositoryInfo loading of repo [%d] failed. Skipping it: %v", repoId, err)
		return nil, err
	}

	if err := repoInfo.loadCommits(ctx, extractCommitsFrom(commits)); err != nil {
		log.Warn("createRepositoryInfo loading of commits of [%d] failed. Skipping it: %v", repoId, err)
		return nil, err
	}

	log.Trace("createRepositoryInfo loading of commits of repo [%d] succeeded", repoId)
	return &repoInfo, nil

}

func extractCommitsFrom(commits []*issue_model.JiraIssueRelatedCommit) []string {
	commitIds := make([]string, 0, len(commits))

	for _, commit := range commits {
		commitIds = append(commitIds, commit.SHA)
	}

	return commitIds
}

func (repositoryInfo *RepositoryInfo) loadCommits(ctx *gitea_context.Context, commits []string) error {
	gitRepo, err := git_repo_module.OpenRepository(ctx, repositoryInfo.Repository)

	if err != nil {
		return err
	}

	defer gitRepo.Close()

	git_commits := gitRepo.GetCommitsFromIDs(commits)
	repositoryInfo.Commits = git_model.ConvertFromGitCommit(ctx, git_commits, repositoryInfo.Repository)

	return nil
}

func (repositoryInfo *RepositoryInfo) loadRepository(ctx *gitea_context.Context, repoId int64) error {
	var err error

	if repositoryInfo.Repository, err = repo_model.GetRepositoryByID(ctx, repoId); err != nil {
		return err
	}

	if repositoryInfo.Repository.IsMirror {
		if repositoryInfo.PullMirror, err = repo_model.GetMirrorByRepoID(ctx, repoId); err != nil {
			return err
		}
	}

	if err := repositoryInfo.Repository.LoadOwner(ctx); err != nil {
		return err
	}
	return nil
}

func initSyncJiraIssuesQueue(ctx context.Context) error {
	syncJiraIssuesQueue = queue.CreateUniqueQueue(ctx, "jira_issues_sync", handlerJiraIssuesSync)
	if syncJiraIssuesQueue == nil {
		return errors.New("unable to create jira_issues_sync queue")
	}
	go graceful.GetManager().RunWithCancel(syncJiraIssuesQueue)

	return nil
}

func handlerJiraIssuesSync(items ...*SyncJiraIssueOptions) []*SyncJiraIssueOptions {
	for _, opts := range items {
		_ = syncJiraIssues(graceful.GetManager().ShutdownContext(), opts.RepoID)
	}
	return nil
}

func syncJiraIssues(ctx context.Context, repoId int64) error {
	repo, err := repo_model.GetRepositoryByID(graceful.GetManager().ShutdownContext(), repoId)
	if err != nil {
		log.Error("syncJiraIssues loading of repo [%d] failed: %v", repoId, err)
		return err
	}

	gitRepo, err := gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		log.Error("syncJiraIssues [repo: %-v]: failed to OpenRepository: %v", repo, err)
		return err
	}

	defer gitRepo.Close()

	return matera_repo_module.SyncIssues(ctx, repo, gitRepo)
}
