# Contributing to terminal-controller-manager

## General Remarks

Contributions to this project are welcome.

Before contributing:

1. Follow the project [Code of Conduct](https://github.com/platform-mesh/.github/blob/main/CODE_OF_CONDUCT.md).
2. Be prepared to accept the Developer Certificate of Origin (DCO) during the pull request process.
3. If you plan a larger feature or architectural change, open an issue first to confirm it fits the project direction.
4. If you use generative AI while contributing, follow the org-wide [guideline for AI-generated code contributions](https://github.com/platform-mesh/.github/blob/main/CONTRIBUTING_USING_GENAI.md).

## How to Contribute

1. Fork the repository and create a branch from `main`.
2. Make your changes.
3. Add or update tests when behavior changes.
4. Verify your changes locally.
5. Open a pull request and address review feedback.

## Development

Prerequisites: Go, Docker, kubectl, kind, and [Task](https://taskfile.dev). See [README.md](README.md) and [docs/CONCEPT.md](docs/CONCEPT.md) for project context.

Key commands:

- `task build` — build the manager binary
- `task run` — run the controller locally
- `task lint` — run formatting and golangci-lint
- `task test` — run the standard local test flow
- `task validate` — run lint and tests
- `task generate` — regenerate CRDs and API resources after API changes
- `task docker-build` — build the controller image
- `task docker-terminal` — build the terminal pod image
- `task docker:kind` — load the controller image into kind and restart the deployment
- `task docker-terminal:kind` — load the terminal image into kind
- `task test-ui` — serve the local websocket test UI

## Pull Requests

- Keep pull requests focused and easy to review.
- Update documentation when APIs, behavior, or workflows change.
- Make sure local verification passes before opening or updating the PR.
- If you change API types, run `task generate` and include the generated output in the same PR.

## License

By contributing, you agree that your contributions will be licensed under the [Apache-2.0 License](LICENSE).
