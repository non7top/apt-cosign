should work against normal repos and those hosted on github via githubusercontent (if that makes and difference)



# Project Specification: `apt-transport-sigstore`
## 1. Executive Summary & GoalThe objective of this project is to build a **fully native APT Transport Method** written in **Go** that brings Sigstore/Cosign verification directly into Debian's `apt` ecosystem.

By registering a custom URL scheme (`sigstore+https://`), this native binary will intercept metadata/package retrieval requests from `apt`, download the targets along with their associated Sigstore artifacts, and enforce strict, highly configurable matching policies against **Rekor transparency log entries** and **OIDC certificate identity claims** before passing them back to `apt`.
---## 2. System Architecture & Flow

[ APT Core Engine ]
│ ▲
│ │ (IPC via Stdin/Stdout using RFC 822 text protocols)
▼ │
[ /usr/lib/apt/methods/sigstore+https ] ───(Reads Config)───> [ /etc/apt/apt.conf.d/ ]
│
├─► 1. Fetches: target file + signature + cert
├─► 2. Validates: cosign signature bundle
└─► 3. Evaluates: Rekor / OIDC metadata assertions against config parameters


---

## 3. Technical Requirements & Wire Protocol

### Implementation Language
* **Go (1.21+)** (Chosen for native compatibility with the official `://github.com` SDK and easy static binary compilation).

### APT IPC Wire Protocol (Standard Input / Output)
The binary must run an infinite loop parsing standard input line-by-line and responding instantly to standard output. **All messages must end with a blank newline (`\n\n`)**.

1. **Handshake on Startup**
   * APT starts the binary and waits for capabilities.
   * **Plugin Response:**
     ```http
     100 Capabilities
     Version: 1.2
     Single-Instance: true
     Pipeline: true
     Send-Config: true

     ```

2. **Configuration Ingestion**
   * Because `Send-Config: true` is set, APT will dump the system's entire configuration tree via stdin right after the handshake using `101 Configuration` blocks. The binary must parse these to locate policy rules.

3. **The File Acquisition Loop**
   * **APT Sends Command:**
     ```http
     600 URI Acquire
     URI: sigstore+https://example.com
     Filename: /var/lib/apt/lists/partial/example.com_dists_stable_InRelease

     ```
   * **Plugin Process:**
     1. Convert `sigstore+https://` to a standard `https://` endpoint.
     2. Download the targeted payload (`InRelease`).
     3. Download the accompanying signature bundle (e.g., `InRelease.sigstore` or custom extension mapping).
     4. Use the Cosign SDK to cryptographically verify the payload.
     5. Parse the resulting Rekor/OIDC JSON payload and check fields against the active ruleset.
   * **Plugin Success Response:**
     ```http
     201 URI Done
     URI: sigstore+https://example.com
     Filename: /var/lib/apt/lists/partial/example.com_dists_stable_InRelease
     Size: 4096

     ```
   * **Plugin Failure Response (if verification or policy fails):**
     ```http
     400 URI Failure
     URI: sigstore+https://example.com
     Message: Sigstore Policy Enforce Failure: Rekor Log Index too low or OIDC mismatch.

     ```

---

## 4. Configuration Schema
The plugin must scan incoming `101 Configuration` blocks (or parse system `/etc/apt/apt.conf.d/` targets) for the following configuration tree:

```apt
// /etc/apt/apt.conf.d/99sigstore-policy
Acquire::sigstore::RekorServer "https://sigstore.dev";

// Core enforcement engine rules
Acquire::sigstore::Enforce::Fields {
    rekorLogIndex "100000";
    certificate-oidc-issuer "https://githubusercontent.com";
    certificate-identity "https://github.com";
};
```

---

## 5. Development Milestones & Implementation Tasks

### Task 1: Protocol Skeleton & Mock Engine
* Set up a Go application project loop reading `os.Stdin` via a scanner.
* Properly handle `100 Capabilities` and echo back proper responses.
* Log all incoming configurations and acquisition hooks to a temporary file (`/tmp/apt-sigstore-debug.log`) for inspection during runtime testing.

### Task 2: Downloader Engine
* Standardize the mapping of incoming `sigstore+https://` blocks to vanilla standard `net/http` fetching actions.
* Ensure streaming handling directly into the path defined by APT’s incoming `Filename:` parameter.

### Task 3: Cosign SDK & Claims Engine Integration
* Pull in `://github.com/pkg/cosign`.
* Implement a validation function that takes the payload data, signature bundle, and evaluates them.
* Use standard JSON parsing tools to unwrap the Rekor bundle array object so individual elements can be checked against string match values gathered in Task 1.

### Task 4: Packaging and Verification Scripts
* Provide a basic `Makefile` targeting clean deployment setups:
  ```makefile
  build:
      go build -o sigstore+https main.go
  install:
      cp sigstore+https /usr/lib/apt/methods/sigstore+https
  ```

---

## 6. How to Test the Implementation

An execution test suite can be run by simulating what `apt` does manually using a local mock config:

```bash
# 1. Compile the agent's work
go build -o sigstore+https main.go

# 2. Feed a simulated manual command into the binary via pipe
echo -e "100 Capabilities\n\n600 URI Acquire\nURI: sigstore+https://debian.org\nFilename: ./TestInRelease\n\n" | ./sigstore+https
```

Ensure the terminal correctly blocks, downloads, logs metadata status information, and prints a well-formatted standard `201 URI Done` or `400 URI Failure` line ending cleanly in a trailing newline sequence.

------------------------------
