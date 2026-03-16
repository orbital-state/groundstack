# groundstack
groundstack is a down to earth cost-free cloud API substitute for testing, prototyping, development and more

## Docs
- Project design (turquoise): [doc/design.md](doc/design.md)

## Testing

Run the Go unit tests with:

- `go test ./...`

This unit-test command only targets Go packages in the repository, such as `internal/...` and `cmd/...`. It does not invoke the Docker Compose and Terraform/example workflows under `examples/**` or the helper scripts under `tests/`.

For the Docker/example flows that act more like integration or usage tests, see:

- [examples/README.md](examples/README.md)
- `tests/run-basic-uc-test.sh`
