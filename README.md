# protoc-gen-go-implement

[中文文档](README_ZH.md)

`protoc-gen-go-implement` is a Go `protoc` plugin that generates empty gRPC service implementations returning `Unimplemented`. It supports single-file and multi-file layouts. Use `package_suffix` to emit implementations in a separate package (dot-importing the proto package), and it works with `paths=source_relative`.

## Install

- Direct:
  ```bash
  go install github.com/nemo1105/protoc-gen-go-implement@latest
  ```
- Local build:
  ```bash
  go build -o ./bin/protoc-gen-go-implement .
  export PATH="$(pwd)/bin:${PATH}"
  ```

## Usage

Plugin name: `protoc-gen-go-implement`. Use alongside `protoc-gen-go` and `protoc-gen-go-grpc`.  
`package_suffix` controls the implementation package (empty = same as proto). Output paths are fully controlled by the `out` argument; works with `paths=source_relative`.

### With protoc

#### Single file (default)
```bash
protoc \
  -I . \
  --go_out=./gen --go-grpc_out=./gen \
  --go-implement_out=paths=source_relative,package_suffix=implement:./gen \
  path/to/your.proto
```
Generates `<prefix>_implement.pb.go` with empty stubs.  
Output location is determined by `out` and `paths`; the example above uses `package_suffix=implement` so files land under `./gen/implement`.

#### Multi file
```bash
protoc \
  -I . \
  --go_out=./gen --go-grpc_out=./gen \
  --go-implement_out=paths=source_relative,layout=multi,package_suffix=implement:./gen \
  path/to/your.proto
```
- Service file: `<prefix>_implement_services.pb.go` (structs and interface assertions).
- RPC files: `<prefix>_<service>_<method>_rpc.pb.go`, one per method, returning `Unimplemented`.
- Output path follows `out` + `paths`; `package_suffix` sets the implementation package name.

### Options (`--go-implement_out=<options>:<out_dir>`)
- `single_suffix`: suffix for single-file layout, default `_implement.pb.go`.
- `services_suffix`: service definition file suffix in multi layout, default `_implement_services.pb.go`.
- `rpc_suffix`: RPC file suffix in multi layout, default `_rpc.pb.go`.
- `impl_suffix`: suffix appended to generated service struct names, default `Impl`.
- `package_suffix`: suffix appended to the proto `go_package` for the implementation package (empty = same package; non-empty will dot-import the proto package).
- `paths`: `import` (default) or `source_relative`.

### With Buf (recommended)
The `example/` directory contains a complete Buf setup:
- `buf.yaml`: lint/breaking config and proto roots.
- `buf.gen.yaml`: generation template (go, go-grpc, go-implement) using multi-file, `paths=source_relative`, `package_suffix=implement`, outputting to `gen/implement`.
- `proto/*`: sample protos.

Steps:
1) Install Buf: https://buf.build/docs/installation (e.g., `brew install buf`).
2) Install generators:
   ```bash
   go install github.com/nemo1105/protoc-gen-go-implement@latest
   ```
3) Generate:
   ```bash
   cd example
   buf generate
   ```
   Outputs to `example/gen` (pb/grpc) and `example/gen/implement` (go-implement, multi-file).

Adjust layout/package: edit `example/buf.gen.yaml` options for the go-implement plugin (e.g., switch to single file or change `package_suffix`).
