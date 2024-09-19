package repository

import (
	"context"
	"fmt"

	"code.gitea.io/gitea/models/db"
	jira_issue_models "code.gitea.io/gitea/models/matera/jira"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/log"
	matera_git_module "code.gitea.io/gitea/modules/matera/git"
)

func DeleteIssues(ctx context.Context, repo *repo_model.Repository) error {
	log.Debug("DeleteIssues: in Repo[%d:%s/%s]", repo.ID, repo.OwnerName, repo.Name)
	err := db.WithTx(ctx, func(ctx context.Context) error {
		affected, err := db.Delete[jira_issue_models.JiraIssueRelatedCommit](ctx, jira_issue_models.FindJiraIssueRelatedCommitOptions{
			RepoID: repo.ID,
		})
		log.Trace("DeleteIssues: done deleting %d issues-related commits in Repo[%d:%s/%s]", affected, repo.ID, repo.OwnerName, repo.Name)
		return err
	})
	return err
}

func SyncIssues(ctx context.Context, repo *repo_model.Repository, gitRepo *git.Repository) error {
	log.Debug("SyncIssues: in Repo[%d:%s/%s]", repo.ID, repo.OwnerName, repo.Name)

	repoCommits, numRepoCommits, err := matera_git_module.GetIssueRelateCommits(gitRepo, 0, 0)

	if err != nil {
		return err
	}

	err = db.WithTx(ctx, func(ctx context.Context) error {
		dbCommits, err := db.Find[jira_issue_models.JiraIssueRelatedCommit](ctx, jira_issue_models.FindJiraIssueRelatedCommitOptions{
			RepoID: repo.ID,
		})

		if err != nil {
			return fmt.Errorf("unable to FindJiraIssueRelatedCommits in pull-mirror Repo[%d:%s/%s]: %w", repo.ID, repo.OwnerName, repo.Name, err)
		}

		inserts, deletes := calcSync(repoCommits, dbCommits)

		for _, commit := range inserts {
			commit.RepoID = repo.ID

			if err := db.Insert(ctx, commit); err != nil {
				return fmt.Errorf("unable insert issue related commit %s for pull-mirror Repo[%d:%s/%s]: %w", commit.SHA, repo.ID, repo.OwnerName, repo.Name, err)
			}
		}

		if len(deletes) > 0 {
			if _, err := db.GetEngine(ctx).Where("repo_id=?", repo.ID).
				In("id", deletes).
				Delete(&jira_issue_models.JiraIssueRelatedCommit{}); err != nil {
				return fmt.Errorf("unable to delete issue related commits for pull-mirror Repo[%d:%s/%s]: %w", repo.ID, repo.OwnerName, repo.Name, err)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("unable to rebuild jira issues table for pull-mirror Repo[%d:%s/%s]: %w", repo.ID, repo.OwnerName, repo.Name, err)
	}

	log.Trace("SyncIssues: done rebuilding %d issues-related commits", numRepoCommits)
	return nil

}

func calcSync(repoCommits []*jira_issue_models.JiraIssueRelatedCommit, dbCommits []*jira_issue_models.JiraIssueRelatedCommit) ([]*jira_issue_models.JiraIssueRelatedCommit, []int64) {
	repoCommitsMap := mappingOf(repoCommits)
	dbCommitsMap := mappingOf(dbCommits)

	inserted := make([]*jira_issue_models.JiraIssueRelatedCommit, 0, 10)
	deleted := make([]int64, 0, 10)

	for _, commit := range repoCommits {
		rel := dbCommitsMap[commit.GetKey()]
		if rel == nil {
			inserted = append(inserted, commit)
		}
	}

	for _, commit := range dbCommits {
		if repoCommitsMap[commit.GetKey()] == nil {
			deleted = append(deleted, commit.ID)
		}
	}

	return inserted, deleted
}

func mappingOf(commits []*jira_issue_models.JiraIssueRelatedCommit) map[jira_issue_models.JiraIssueRelatedCommitKey]*jira_issue_models.JiraIssueRelatedCommit {
	commitMap := make(map[jira_issue_models.JiraIssueRelatedCommitKey]*jira_issue_models.JiraIssueRelatedCommit)

	for _, commit := range commits {
		commitMap[commit.GetKey()] = commit
	}

	return commitMap
}
