# protoc-gen-go-implement

[中文文档](README_zh.md)

`protoc-gen-go-implement` is a Go `protoc` plugin that generates empty gRPC/Connect service implementations returning “not implemented” errors. It supports single-file or multi-file output so you can quickly scaffold services or split per-method placeholders. Use `package_suffix` to emit implementations in a separate package, choose gRPC or Connect via `target`, and it works with `paths=source_relative`.

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

Plugin name: `protoc-gen-go-implement`. Use alongside `protoc-gen-go` and `protoc-gen-go-grpc` or `protoc-gen-connect-go`.  
`target` picks gRPC (default) or Connect. `package_suffix` controls the implementation package (empty = same as proto). Output paths are fully controlled by the `out` argument; works with `paths=source_relative`.

### With protoc

#### Single file (default)
```bash
protoc \
  -I . \
  --go_out=./gen --go-grpc_out=./gen \
  --go-implement_out=target=grpc,paths=source_relative,package_suffix=implement:./gen \
  path/to/your.proto
```
Generates `<prefix>_implement.pb.go` with empty stubs.  
Output location is determined by `out` and `paths`; the example above combines `package_suffix=implement` to produce package `implement` under `./gen/implement`.

#### Multi file
```bash
protoc \
  -I . \
  --go_out=./gen --go-grpc_out=./gen \
  --go-implement_out=target=grpc,paths=source_relative,layout=multi,package_suffix=implement:./gen \
  path/to/your.proto
```
- Service file: `<prefix>_implement_services.pb.go` (structs and interface assertions).
- RPC files: `<prefix>_<service>_<method>_rpc.pb.go`, one per method, returning `Unimplemented`.
- Output path follows `out` + `paths`; `package_suffix` sets the implementation package name.

For Connect, swap in `--connect-go_out` and set `target=connect` (align `connect_package_suffix` with your connect-go `package_suffix` if customized):
```bash
protoc \
  -I . \
  --go_out=./gen --connect-go_out=./gen \
  --go-implement_out=target=connect,paths=source_relative,layout=multi,package_suffix=implement,connect_package_suffix=connect:./gen \
  path/to/your.proto
```

### Options (`--go-implement_out=<options>:<out_dir>`)
- `target=grpc|connect`: choose gRPC (default) or Connect handlers.
- `single_suffix`: suffix for single-file layout, default `_implement.pb.go`.
- `services_suffix`: service definition file suffix in multi layout, default `_implement_services.pb.go`.
- `rpc_suffix`: RPC file suffix in multi layout, default `_rpc.pb.go`.
- `impl_suffix`: suffix appended to generated service struct names, default `Impl`.
- `package_suffix`: suffix appended to the proto `go_package` for the implementation package (empty = same package).
- `connect_package_suffix`: suffix for the Connect-generated package (default `connect`, matches `protoc-gen-connect-go` default).
- `paths`: `import` (default) or `source_relative`.

### With Buf (recommended)
The `example/` directory contains a complete Buf setup:
- `buf.yaml`: lint/breaking config and proto roots.
- `buf.gen.yaml`: generation template.
- `proto/*`: sample proto files.

Steps:
1) Install Buf: https://buf.build/docs/installation (e.g., `brew install buf`).
2) Install generator:
   ```bash
   go install github.com/nemo1105/protoc-gen-go-implement@latest
   ```
3) Generate:
   ```bash
   cd example
   buf generate
   ```
   See the directory layout after generation in the comment at the bottom of `example/buf.gen.yaml`.
