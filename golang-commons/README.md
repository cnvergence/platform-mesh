# Platform Mesh - golang-commons
![Build Status](https://github.com/platform-mesh/golang-commons/actions/workflows/pipeline.yml/badge.svg)
[![REUSE status](
https://api.reuse.software/badge/github.com/platform-mesh/golang-commons)](https://api.reuse.software/info/github.com/platform-mesh/golang-commons)
## Description

golang-commons contains golang library packages to be reused across microservices and operators/controllers. The scope includes, but is not limited to:

- JWT handling
- Context handling
- Error handling
- Logging
- [Controllers](./controller/README.md)

## Getting started

Add the dependency to your go module based project like so:

```
go get github.com/platform-mesh/golang-commons
```

## Releasing

The release is performed automatically through a GitHub Actions Workflow.

All the released versions will be available through access to GitHub (as any other Golang library).

## Requirements

golang-commons requires a installation of go. Checkout the [go.mod](go.mod) for the required go version.


## Contributing

Please refer to the [CONTRIBUTING.md](CONTRIBUTING.md) file in this repository for instructions on how to contribute to Platform Mesh.

## Code of Conduct

Please refer to our [Code of Conduct](https://github.com/platform-mesh/.github/blob/main/CODE_OF_CONDUCT.md) for information on the expected conduct for contributing to Platform Mesh.