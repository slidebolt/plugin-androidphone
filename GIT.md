# Git Workflow for plugin-androidphone

This repository contains the Slidebolt Android Phone Plugin, providing integration with Android devices via FCM (Firebase Cloud Messaging). It produces a standalone binary.

## Dependencies
- **Internal:**
  - `sb-api`: API gateway for receiving device registration.
  - `sb-contract`: Core interfaces.
  - `sb-domain`: Shared domain models.
  - `sb-messenger-sdk`: Shared messaging interfaces.
  - `sb-runtime`: Core execution environment.
  - `sb-storage-sdk`: Shared storage interfaces.
  - `sb-testkit`: Testing utilities.
- **External:** 
  - `golang.org/x/oauth2`: Google OAuth2 support for FCM.

## Build Process
- **Type:** Go Application (Plugin).
- **Consumption:** Run as a background plugin service.
- **Artifacts:** Produces a binary named `plugin-androidphone`.
- **Command:** `go build -o plugin-androidphone ./cmd/plugin-androidphone`
- **Validation:** 
  - Validated through unit tests: `go test -v ./...`
  - Validated by successful compilation of the binary.

## Pre-requisites & Publishing
As an Android integration plugin, `plugin-androidphone` must be updated whenever the core API, domain, messaging, storage, or testkit SDKs are changed.

**Before publishing:**
1. Determine current tag: `git tag | sort -V | tail -n 1`
2. Ensure all local tests pass: `go test -v ./...`
3. Ensure the binary builds: `go build -o plugin-androidphone ./cmd/plugin-androidphone`

**Publishing Order:**
1. Ensure all internal dependencies are tagged and pushed.
2. Update `plugin-androidphone/go.mod` to reference the latest tags.
3. Determine next semantic version for `plugin-androidphone` (e.g., `v1.0.5`).
4. Commit and push the changes to `main`.
5. Tag the repository: `git tag v1.0.5`.
6. Push the tag: `git push origin main v1.0.5`.

## Update Workflow & Verification
1. **Modify:** Update Android integration logic in `app/` or FCM logic in `fcm.go`.
2. **Verify Local:**
   - Run `go mod tidy`.
   - Run `go test ./...`.
   - Run `go build -o plugin-androidphone ./cmd/plugin-androidphone`.
3. **Commit:** Ensure the commit message clearly describes the Android plugin change.
4. **Tag & Push:** (Follow the Publishing Order above).
