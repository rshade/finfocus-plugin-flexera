# Version Management

This project includes a comprehensive version management system that provides detailed information about the binary version, build details, and git information.

## Version Information

The version system provides the following information:

- **Version**: Semantic version of the application
- **Build Date**: When the binary was compiled
- **Git Commit**: Short git commit hash
- **Git Branch**: Current git branch name
- **Git State**: Whether the repository is clean or dirty
- **Go Version**: Go runtime version used for compilation
- **Platform**: Target OS and architecture

## Usage

### Command Line Flags

The binary supports version-related command line flags:

```bash
# Show basic version information
./pulumicost-kubecost -version

# Show detailed version information
./pulumicost-kubecost -version-full
```

### Programmatic Usage

```go
import "github.com/rshade/pulumicost-plugin-kubecost/pkg/version"

// Get basic version string
fmt.Println(version.String())

// Get detailed version information
fmt.Println(version.FullString())

// Get structured version info
info := version.GetVersionInfo()
fmt.Printf("Version: %s\n", info.Version)
fmt.Printf("Git Commit: %s\n", info.GitCommit)
```

### Build Time

Version information is injected at build time using ldflags. The Makefile automatically extracts git information and build date:

```bash
# Build with default version (1.0.0)
make build

# Build with custom version
VERSION=2.1.0 make build

# Show version information before building
make version-info
```

## Makefile Targets

- `make build`: Build the binary with version information
- `make version`: Show current version variables
- `make version-info`: Show detailed version information
- `make clean`: Clean build artifacts

## Version Variables

The following variables can be set during build:

- `VERSION`: Semantic version (default: 1.0.0)
- `GIT_COMMIT`: Git commit hash (auto-detected)
- `GIT_BRANCH`: Git branch name (auto-detected)
- `GIT_STATE`: Git repository state (auto-detected)
- `BUILD_DATE`: Build timestamp (auto-generated)

## Example Output

### Basic Version String
```
v1.0.0 (a1b2c3d, 2024-01-15_14:30:00_UTC, linux/amd64)
```

### Full Version Information
```
Version: 1.0.0
Build Date: 2024-01-15_14:30:00_UTC
Git Commit: a1b2c3d
Git Branch: main
Git State: clean
Go Version: go1.24.5
Platform: linux/amd64
```

## Testing

Run the version tests:

```bash
go test ./pkg/version
```

## Integration

The version information is automatically logged when the application starts:

```
2024/01/15 14:30:00 pulumicost-kubecost starting, v1.0.0 (a1b2c3d, 2024-01-15_14:30:00_UTC, linux/amd64)
```
