# ARIES: Agent Runtime & Infrastructure Experimentation System

ARIES is an open-source experimentation framework for agent serving systems.
It lets systems researchers run reproducible agent benchmarks while observing
the full task trajectory: repeated model calls, harness decisions, stateful tool
execution, task outcome, and available resource telemetry.

Agent workloads are not isolated LLM requests. An agent repeatedly observes its
state, invokes a model, executes tools in a sandbox, and incorporates the
results into the next step. ARIES treats that ordered, end-to-end trajectory as
the unit of experimentation, so researchers can relate task progress and
correctness to behavior across the agent-serving stack.

## Mission

ARIES aims to help researchers innovate across the software and systems stack
that serves AI agents. Conventional model-serving measurements—such as token
throughput or per-request latency—do not explain delays, resource pressure, or
failures that occur in agent harnesses and tool sandboxes. ARIES bridges that
measurement gap by keeping task semantics independent from the execution stack,
making stateful tool execution consistently observable, and preserving evidence
needed to reconstruct a run.

The framework supports controlled comparisons across agent harnesses, model
backends, and sandbox substrates without changing the benchmark task being
evaluated. It is designed for experiments that connect task success with
end-to-end latency, resource use, and execution behavior—not only model-call
metrics.

## Why ARIES

ARIES is built around three needs of agent-serving research:

- **Preserve task semantics across configurations.** Benchmark tasks and their
evaluation stay separate from the chosen harness, model backend, tool bridge,
sandbox, and telemetry setup.
- **Observe complete agent trajectories.** Run artifacts and correlated
component evidence make it possible to study where an agent spends time and
how execution behavior relates to the final outcome.
- **Make stateful tool execution comparable.** A narrow bridge gives the
harness temporary access to a persistent task environment while sandbox
adapters retain control of lifecycle, isolation, and resource observation.
- **Ground systems research in production behavior.** The included
 [Ant Group Agentic LLM Trace 2026](docs/ant-group-agent-LLM-trace.md) captures
engine request logs and harness-environment metrics from a real online
serving workload.

This enables research on questions such as whether tool and harness work—not
inference alone—limits task completion; how retained trajectory context trades
accuracy for serving capacity; and how sandbox resource management and
isolation should evolve for long-running agents.

## ARIES architecture

![design](docs/_figures/aries.png)

ARIES separates benchmark task meaning from execution configuration. For every
task, the Runner composes four substitutable roles:

| Role | Responsibility |
| --- | --- |
| `Benchmark` | Loads tasks, keeps verifier material private, prepares the live sandbox, and independently evaluates final task state. |
| `AgentHarness` | Runs the configured agent and its model interaction. |
| `ToolSandbox` | Owns the isolated task environment and confirms its cleanup. |
| `ToolBridge` | Grants one harness temporary, narrow access to one sandbox and positively revokes it. |

The model service and recorder surround this per-task composition. A model
endpoint may be external or, for SGLang, managed for a profile run; neither is a
fifth Runner role.

The lifecycle is deliberately fail-closed: ARIES stops the harness and confirms
that the bridge has been revoked before exposing private verifier material for
evaluation. Evaluation follows each benchmark's original methodology, against
the still-live sandbox or in a fresh sandbox from the task image, and remains
separate from the harness outcome; ARIES then removes the sandboxes. This keeps agent
execution, tool access, and scoring under distinct ownership while retaining
replayable private evidence for a run.

Read the [design principles](docs/design.md) for lifecycle, isolation, measurement,
and extension requirements. Detailed [component contracts](docs/design.md#guides)
and [implementation explanations](docs/implementation/README.md) cover the code's
current behavior. Coding agents start at [AGENTS.md](AGENTS.md).

For a Chinese introduction with component diagrams, a complete task walkthrough,
and the cloud-sandbox implementation boundaries, read the [beginner architecture guide](docs/architecture-guide.md).

## Current implementation

The research framework is intended to support multiple harnesses, benchmarks,
and sandbox substrates. This repository currently wires the following explicit
implementations for the original `aries PROFILE.json` command:

| Role or service | Implementation |
| --- | --- |
| Agent harness | OpenClaw (text and realtime voice modes); Hermes (text) |
| Benchmark | Terminal-Bench 2; Deep Research Bench; SWE-Atlas QA; the 731-task public SWE-bench Pro split; Toolathlon |
| Tool sandbox | Shared Docker deployment through the Moby Go SDK; Kubernetes pods through `kubectl` |
| Tool bridge | One managed SSH service per run, with OpenClaw or Hermes dialect and independent sandbox sessions |
| Model service | External DeepSeek; external OpenAI-compatible servers such as vLLM; external or ARIES-managed SGLang |

See [Supported implementations](docs/supported.md) for ownership,
configuration keys, status, and runnable profiles. ARIES uses explicit
constructors and command switches; it does not discover or register components
at runtime.

### E2B coding workloads: experimental cloud path

`aries cloud [--env-file PATH] [--tape FILE] PROFILE.json` adds a separate,
bounded E2B workload while reusing the four Runner roles. OpenClaw runs either
in an ECS Docker container (**A: external**) or inside the E2B task sandbox
(**B: embedded**). Both execute real tools against a Python coding task;
an independent verifier checks the captured candidate in a fresh sandbox.

The [five cloud profiles](profiles/cloud/) cover A/B real-model recording,
A/B strict replay with recorded timing, and B replay without added model wait.
Replay replaces model responses while preserving real Agent/tool execution;
A and B use their own recordings. Per-attempt bearer tokens bind requests at
the fixed `/v1` model endpoint to separate replay sessions. This path is for
sandbox workload experiments, not a comparison of LLM capability.

All five baseline conditions have passed one real E2B run each, including
independent verification, positive isolation and confirmed cleanup. Three further
repeated/concurrent batches, bounded lifecycle fault probes and isolated Replay
process measurements are documented in the [architecture guide](docs/architecture-guide.md#84-重复并发和故障路径的实际结果).
A suitable E2B template
with confirmed cgroup isolation and a reachable HTTPS model origin are prerequisites.
Cumulative creates are recorded without a hard limit; concurrency, TTL, ownership,
and confirmed cleanup remain enforced.
Follow the [ECS deployment guide](deploy/e2b/README.md),
[cloud configuration](docs/configuration.md#e2b-cloud-workload-profiles), and
[strict replay contract](docs/implementation/model-replay.md) before execution.
[Huawei AgentSphere](deploy/agentsphere/README.md) uses a separate Adapter and
provider configuration. Its A recording and B recording/replay passed independent
verification under an explicit, unverified verifier-egress assumption. The new
`merge-ranges-portable-v1` A workload also passed one real E2B recording followed
by strict recorded-timing replays of that unchanged tape on E2B and Huawei:
eight model turns, seven real tool calls, and 12 independent tests in each run.
Stop/revocation and cleanup were confirmed. These single attempts establish a
[conditional same-tape comparison](docs/performance-archive.md#portable-crosscloud),
not provider capacity or a performance ranking. The legacy tape's filesystem
metadata divergence remains a failed experiment; Huawei no-egress enforcement
remains unverified.

The approved observability work is tracked in the [P0 implementation report](docs/observability/OBSERVABILITY_IMPLEMENTATION_REPORT.md),
with [R01–R18 evidence and remaining blockers](docs/observability/OPEN_ISSUES_AND_RESOLUTIONS.md).
A readable trace does not establish complete Tool identity coverage.

## Getting started

Build, test, deploy, and run experiments only on the designated **remote Linux
ECS**; the workstation is for source editing and static review. In the original
Docker profile path, harness and sandbox placement use independent `deployment`
blocks and require the same Docker daemon local to that ECS. **Remote Docker
servers and arbitrary mixed deployment backends are not supported.** See
[deployment configuration](docs/configuration.md#deployment-configuration).

ARIES requires Linux, a local Docker Engine, Go, Git, Make, network access to
the configured model service, and access to required image registries.

```sh
make bridge-image
./bin/aries profiles/openclaw-tb2-fix-git-deepseek.json
```

Before a run, ARIES idempotently prepares the pinned benchmark checkout and
required container images. `aries setup PROFILE.json` is available to prewarm
those inputs; it does not start a managed runtime, load model weights, or
contact an external model endpoint.

The DeepSeek example requires an API key and can incur charges. The
[Quick start](docs/quick-start.md) covers secure credential setup, the first
run, SGLang alternatives, result inspection, and troubleshooting.

To run on a Kubernetes cluster built with Invitro's setup scripts, see
[`k8s/`](k8s/README.md).

For repository-level software-engineering tasks, see the
[SWE-bench Pro guide](docs/benchmarks/swe-bench-pro.md). Its setup additionally
requires Git LFS and access to the pinned public dataset, evaluator repository,
and selected `linux/amd64` task images.

## Reproducing the paper

The [`profiles/experiments/`](profiles/experiments/) directory contains the profiles used to
reproduce the results of the paper: twenty-task Terminal-Bench 2, SWE-Bench
Pro, and Deep Research Bench runs for both OpenClaw and Hermes, at concurrency 1 with no container
resource limits. Run them from the repository root, for example
`./bin/aries profiles/experiments/openclaw-tb2-twenty-deepseek.json`.

## Trace dataset

The repository includes the [Ant Group Agentic LLM Trace 2026](docs/ant-group-agent-LLM-trace.md)
under `traces/` — per-pod 24-hour engine logs, a working-hours engine log from
a randomly selected group of pods, and harness CPU/memory utilization metrics
collected from Ant Group's production inference infrastructure. The dataset is
released under [CC-BY-4.0](LICENSE) and is tracked with Git LFS; fetch the
files after cloning with `git lfs pull`.

## Research and roadmap

ARIES accompanies the paper [*Rethinking AI Cloud Infrastructure for Agentic
Serving Systems with the Aries Experimentation Framework*](https://arxiv.org/abs/2607.29069).
The paper uses ARIES alongside anonymized production traces to study agent
serving beyond token-centric metrics, including harness and tool critical-path
costs, context-capacity trade-offs, bursty sandbox resources, and sandbox attack
surface.

[Aries Roadmap](https://github.com/orgs/hyscale-lab/projects/7)

Future work may add rigorously tested implementations behind the existing
component boundaries and broaden repeatable experiment workflows. Those are
research directions, not claims of currently supported integrations.

## Industry collaborators

ARIES is built and maintained in collaboration with Amazon Web Services,
Microsoft, AMD Singapore, Ant Group, and NCSpeech.

## Community, contributing, and contact

Contributions are welcome through [GitHub issues](https://github.com/hyscale-lab/ARIES/issues)
and pull requests. Questions and design proposals can use
the issue tracker so the discussion remains available to the community.

## Maintainers
### Agent Harness, Benchmark, Tool Sandbox, Tool Bridge
- JooYoung Park (jooyoung001 at e.ntu.edu.sg)
- Leonid Kondrashov (leonid001 at e.ntu.edu.sg)

### GPU, Industry Traces
- Chengzhi Lu (chengzhi.lu at ntu.edu.sg)

## Citation

```bibtex
@misc{kondrashov2026rethinkingaicloudinfrastructure,
title={Rethinking AI Cloud Infrastructure for Agentic Serving Systems with the Aries Experimentation Framework},
author={Leonid Kondrashov and Hongrui Liu and JooYoung Park and Boxi Zhou and Zonghao Liu and Chengzhi Lu and Riccardo Mancini and Esha Choukse and Haris Javaid and German Sviridov and Tao Peng and Chen Zhao and Anastasia Avdeeva and Aleksei Gusev and Marios Kogias and Luo Mai and Dmitrii Ustiugov},
year={2026},
eprint={2607.29069},
archivePrefix={arXiv},
primaryClass={cs.DC},
url={https://arxiv.org/abs/2607.29069},
}
```

## License

ARIES code base is licensed under the [MIT License](LICENSE-CODE).

The trace from Ant Group is under the [CC-BY-4.0](LICENSE).
