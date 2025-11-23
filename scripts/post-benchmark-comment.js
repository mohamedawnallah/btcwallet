#!/usr/bin/env node

// post-benchmark-comment.js
// Posts or updates a PR comment with benchmark results

const fs = require('fs');

async function postComment(github, context) {
  const summaryFile = process.env.SUMMARY_FILE || 'summary.txt';

  if (!fs.existsSync(summaryFile)) {
    console.log(`Summary file ${summaryFile} not found, skipping PR comment`);
    return;
  }

  const summary = fs.readFileSync(summaryFile, 'utf8');

  // Find existing benchmark comment
  const comments = await github.rest.issues.listComments({
    owner: context.repo.owner,
    repo: context.repo.repo,
    issue_number: context.issue.number,
  });

  const botComment = comments.data.find(comment =>
    comment.user.type === 'Bot' &&
    comment.body.includes('📊 Benchmark Results')
  );

  if (botComment) {
    console.log('Updating existing benchmark comment');
    await github.rest.issues.updateComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      comment_id: botComment.id,
      body: summary
    });
  } else {
    console.log('Creating new benchmark comment');
    await github.rest.issues.createComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      issue_number: context.issue.number,
      body: summary
    });
  }
}

module.exports = postComment;

// If running standalone (for testing)
if (require.main === module) {
  console.log('This script is meant to be used with github-script action');
  process.exit(1);
}
