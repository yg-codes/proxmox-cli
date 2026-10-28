# Third-Party Notices and Licenses

This project's release archives and packages distribute third-party Go
modules as statically linked components of the compiled binaries. The
complete license text for every dependency is included in this repository
under [`third_party_licenses/`](third_party_licenses/) and is shipped inside
every release archive and native package.

## Dependencies

| Dependency | Version | License | Full text |
|---|---|---|---|
| `github.com/sirupsen/logrus` | v1.9.3 | MIT | `third_party_licenses/sirupsen-logrus-MIT.txt` |
| `github.com/spf13/cobra` | v1.8.0 | Apache-2.0 | `third_party_licenses/spf13-cobra-Apache-2.0.txt` |
| `github.com/spf13/pflag` | v1.0.5 | BSD-3-Clause | `third_party_licenses/spf13-pflag-BSD-3-Clause.txt` |
| `github.com/stretchr/testify` | v1.8.4 | MIT | `third_party_licenses/stretchr-testify-MIT.txt` |
| `golang.org/x/sys` | v0.15.0 | BSD-3-Clause | `third_party_licenses/golang-x-sys-BSD-3-Clause.txt` |
| `github.com/davecgh/go-spew` | v1.1.2 | ISC | `third_party_licenses/davecgh-go-spew-ISC.txt` |
| `github.com/inconshreveable/mousetrap` | v1.1.0 | Apache-2.0 | `third_party_licenses/inconshreveable-mousetrap-Apache-2.0.txt` |
| `github.com/pmezard/go-difflib` | v1.0.1 | BSD-3-Clause (BSD-style) | `third_party_licenses/pmezard-go-difflib-BSD-3-Clause.txt` |

All identified licenses are permissive and compatible with this project's
MIT license.

## Redistribution obligations (why the texts are included)

Binary-only distribution still carries license obligations:

- **Apache-2.0 §4(a)-(d)**: recipients of an executable that redistributes
  Apache-licensed material must receive a copy of the Apache License and the
  applicable attribution notices. Cobra and mousetrap are covered by the
  included Apache-2.0 texts.
- **MIT / BSD-3-Clause / ISC / BSD-style**: each license requires that the
  copyright notice and license text be retained in redistributions. The
  included per-dependency files are the upstream license files verbatim,
  with their original copyright lines.

Accordingly, every `.tar.gz`/`.zip` archive, `.deb`, and `.rpm` produced by
the release pipeline embeds `THIRD_PARTY_NOTICES.md` and the full
`third_party_licenses/` directory (see `.goreleaser.yml` — archives
`files:` and nfpm `contents:`).

## Maintenance

If dependencies are added or upgraded, update this table and copy the new
upstream license files into `third_party_licenses/`
(`go list -m all` and each module's LICENSE in the module cache).
