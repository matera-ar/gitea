// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_22 //nolint

import (
	"fmt"

	"code.gitea.io/gitea/modules/timeutil"
	"xorm.io/xorm"
)

func CreateJiraIssueRelatedCommitTable(x *xorm.Engine) error {
	type JiraIssueRelatedCommit struct {
		ID          int64              `xorm:"pk autoincr"`
		Ticket      string             `xorm:"varchar(64) not null index(jira_issue_ticket) index(jira_issue_repo)"`
		RepoID      int64              `xorm:"not null index(jira_issue_ticket)"`
		SHA         string             `xorm:"varchar(64) not null`
		CreatedUnix timeutil.TimeStamp `xorm:"not null index(jira_issue_ticket)"`
	}

	if error := x.Sync(new(JiraIssueRelatedCommit)); error != nil {
		return fmt.Errorf("Error creating jira_issue_related_commit table: %w", error)
	} else {
		return nil
	}
}
