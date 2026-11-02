---
name: deploy-local
description: Deploy the current checkout (main or a worktree) to local Docker for manual testing, and tear it down afterwards. Args - up (default), down, status.
disable-model-invocation: true
---

# deploy-local

Run `.claude/skills/deploy-local/deploy-local.sh $ARGUMENTS` from the checkout being tested (main repo or a `.claude/worktrees/...` worktree), with a timeout of at least 5 minutes for `up`.

- `up` (default): wipes any previous `waystone-local` deployment, builds the current checkout, starts it with a **fresh database**, and waits until http://localhost:8080 responds. Uses the main repo's `.env` when running from a worktree. The fresh DB has only the admin and the game systems list unless that `.env` sets `SEED_DEMO_DATA=1` (demo users `testy`/`roy`/`adam`/`simon`, campaigns and sessions); pass it inline for one deploy with `SEED_DEMO_DATA=1 .claude/skills/deploy-local/deploy-local.sh up`.
- `down`: removes the container, data volume, and image. Nothing is left behind.
- `status`: shows the running container.

All deploys share one compose project (`waystone-local`), so deploying another branch replaces the previous one instead of piling up images and volumes. Don't use `docker compose up` or `./deploy.sh` directly for testing: they name the project after the folder, so each worktree leaves its own image and volume behind.

After `up`, tell the user the URL and remind them to run `/deploy-local down` when finished. If `up` fails, show the log tail it printed and diagnose it. Never read `.env`.
