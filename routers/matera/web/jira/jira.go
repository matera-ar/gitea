package jira

import (
	"net/http"

	"code.gitea.io/gitea/modules/base"
	"code.gitea.io/gitea/services/context"
	jira_service "code.gitea.io/gitea/services/matera/jira"
)

const (
	tplCommitsByJira base.TplName = "matera/jira/jira_issue"
	pageSize         int          = 50
)

func CommitsByJira(ctx *context.Context) {
	page := int(ctx.PathParamInt64("idx"))
	if page <= 1 {
		page = ctx.FormInt("page")
	}

	if page <= 0 {
		page = 1
	}

	jiraIssue, _ := jira_service.FindJiraIssue(ctx, page, pageSize)

	pager := context.NewPagination(jiraIssue.TotalCommits, pageSize, page, 5)
	pager.SetDefaultParams(ctx)

	ctx.Data["JiraIssue"] = jiraIssue
	ctx.Data["Total"] = jiraIssue.TotalCommits
	ctx.Data["Page"] = pager

	ctx.HTML(http.StatusOK, tplCommitsByJira)
}
