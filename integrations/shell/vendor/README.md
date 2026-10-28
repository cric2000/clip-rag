# Bundled Bash line editor

`blesh/` is the unmodified official ble.sh release **20261027+c70c0e4**.

- Upstream: https://github.com/akinomyoga/ble.sh
- Archive: https://github.com/akinomyoga/ble.sh/releases/download/nightly/ble-nightly-20261027%2Bc70c0e4.tar.xz
- SHA-256: `35d184067e8fe84a46a66e60265e736f47a81f80497c1189c7145747d7c21383`
- License: BSD-3-Clause; see `blesh/licenses/LICENSE.md` and the retained
  notices for included components in `blesh/licenses/`.

`../runtime.go` embeds only `ble.sh`, `lib/`, `licenses/` and the default colour
scheme. ClipRAG's adapter lives outside this directory in `../cliprag.bash`.
The helper stages that embedded runtime atomically in the user's cache. It does
not download or update shell code at runtime.
