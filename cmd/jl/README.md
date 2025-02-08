# OpenMFP - jl

jl is a utility cli that can be used to explore json based log files and log streams. It is especially useful when 
exploring kubernetes log streams.

# Installation


## Installation from source
- Checkout this repository
- Use `go install .` from the top-level directory of the clone.

# Usage 

```bash
# Show kubernetes log streams
kubectl logs my-pod -n my-namespace --follow | jl

# Show logs stored in a file
jl view -i data/input.log

# Show logs stored in a file but skip the level and service properties
jl view -i data/input.log -s level,service

# Show logs stored in a file but focus only on the message and reconcile_id property
jl view -i data/input.log -f message,reconcile_id

# Show logs stored in a file but focus only on the message property, display only rows that match the select expression
jl view -i data/input.log -rf message --select=reconcile_id=fcdc26ae-7feb-4bf2-9058-f0c6666bc356
```    

## Code of Conduct

Please refer to the [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) file in this repository informations on the expected Code of Conduct for contributing to OpenMFP.

## Licensing

Copyright 2024 SAP SE or an SAP affiliate company and OpenMFP contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/openmfp/jl).
