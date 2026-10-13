#!/bin/bash
# Local Docker deploy for manual testing. Always uses one fixed compose project
# so repeated deploys (from main or any worktree) never pile up images/volumes.
#   deploy-local.sh up      build + start with a fresh DB (default)
#   deploy-local.sh down    stop and remove container, volume, image
#   deploy-local.sh status  show what's running
set -euo pipefail

PROJECT=waystone-local
URL="http://localhost:${PORT:-8080}"

# Run from the current checkout (main repo or a worktree).
SRC=$(git rev-parse --show-toplevel)
# .env is gitignored, so worktrees don't have one: use the main repo's.
MAIN=$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")
ENV_FILE=""
for f in "$SRC/.env" "$MAIN/.env"; do
  if [ -f "$f" ]; then ENV_FILE=$f; break; fi
done

compose() {
  if [ -n "$ENV_FILE" ]; then
    docker compose -p "$PROJECT" --project-directory "$SRC" --env-file "$ENV_FILE" -f "$SRC/docker-compose.yml" "$@"
  else
    docker compose -p "$PROJECT" --project-directory "$SRC" -f "$SRC/docker-compose.yml" "$@"
  fi
}

cleanup() {
  compose down -v --rmi local --remove-orphans 2>/dev/null || true
  # Old images left dangling by rebuilds of this project.
  docker image prune -f --filter "label=com.docker.compose.project=$PROJECT" >/dev/null
}

case "${1:-up}" in
  up)
    docker info >/dev/null 2>&1 || { echo "Docker is not running." >&2; exit 1; }
    echo "Deploying $(git -C "$SRC" rev-parse --abbrev-ref HEAD) from $SRC"
    if [ -n "$ENV_FILE" ]; then echo "Using env file: $ENV_FILE"
    else echo "WARNING: no .env found - ADMIN_PASSWORD unset, admin can't log in."; fi
    cleanup
    compose up -d --build
    for _ in $(seq 1 30); do
      code=$(curl -s -o /dev/null -w '%{http_code}' "$URL/login.html" || true)
      if [ "$code" = "200" ]; then echo "Up: $URL"; exit 0; fi
      sleep 1
    done
    echo "App did not respond on $URL within 30s. Logs:" >&2
    compose logs --tail 50 >&2
    exit 1
    ;;
  down)
    cleanup
    echo "Removed $PROJECT container, volume and image."
    ;;
  status)
    compose ps
    ;;
  *)
    echo "usage: $0 [up|down|status]" >&2; exit 2
    ;;
esac
