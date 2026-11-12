import { execFileSync } from "node:child_process";

// Keep the repository-owned hook active after dependency installation.
execFileSync("git", ["config", "--local", "core.hooksPath", ".githooks"], {
  stdio: "inherit",
});
