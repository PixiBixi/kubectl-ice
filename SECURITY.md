# Security Policy

## Supported versions

Only the latest release receives fixes. Older tags are not patched.

This repository is a maintained fork of
[NimbleArchitect/kubectl-ice](https://github.com/NimbleArchitect/kubectl-ice).
Report anything that affects the code or the releases published here through
the process below; a vulnerability that only exists upstream belongs to that
repository.

## Reporting a vulnerability

Report privately through GitHub's [security advisory form][advisory]. Please
do not open a public issue for a vulnerability.

[advisory]: https://github.com/PixiBixi/kubectl-ice/security/advisories/new

Expect an acknowledgement within 7 days. If the report is confirmed, the fix
ships in the next release and the advisory is published once it is available.

This is a personal project maintained on a best-effort basis, with no service
level commitment.

## Scope

In scope: the code in this repository and the release artifacts published from
it (archives, checksums, SBOMs, signatures).

Out of scope: vulnerabilities in upstream dependencies, which belong to their
own maintainers, and issues that require an already-compromised local machine or
an already-compromised cluster.

kubectl-ice reads from a cluster and prints what it finds. It holds the
permissions of whoever runs it, through their own kubeconfig, and never writes
to the cluster. A report that amounts to "it displays what the user is already
allowed to read" is not a vulnerability.

## Verifying a release

Releases are signed keylessly with cosign and carry a build provenance
attestation:

```sh
cosign verify-blob \
  --certificate-identity-regexp 'https://github.com/PixiBixi/kubectl-ice/.github/workflows/release.yml@.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  --bundle checksums.txt.sigstore.json \
  checksums.txt

gh attestation verify <archive>.tar.gz --repo PixiBixi/kubectl-ice
```
