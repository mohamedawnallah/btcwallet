#!/usr/bin/env node

// post-benchmark-comment.js
// Posts a PR comment with benchmark results

import { existsSync, readFileSync } from 'fs';

async function postComment(github, context) {
  const summaryFile = process.env.SUMMARY_FILE || 'summary.txt';

  if (!existsSync(summaryFile)) {
    console.log(`Summary file ${summaryFile} not found, skipping PR comment`);
    return;
  }

  const summary = readFileSync(summaryFile, 'utf8');

  console.log('Creating new benchmark comment');
  await github.rest.issues.createComment({
    owner: context.repo.owner,
    repo: context.repo.repo,
    issue_number: context.issue.number,
    body: summary
  });
}

export default postComment;
