{
  description = "A lightweight RSS feed aggregator and reader";

  inputs = {
    flake-utils.url = "github:numtide/flake-utils";

    # 1.23.2 release
    # We temporarily mismatch the version in Docker (1.23.3) because Nix doesn't
    # have the latest version yet, but we need to fix:
    # https://github.com/golang/go/issues/68976
    go-nixpkgs.url = "github:NixOS/nixpkgs/4ae2e647537bcdbb82265469442713d066675275";

    # 3.44.2 release
    sqlite-nixpkgs.url = "github:NixOS/nixpkgs/5ad9903c16126a7d949101687af0aa589b1d7d3d";

    # 20.6.1 release
    nodejs-nixpkgs.url = "github:NixOS/nixpkgs/78058d810644f5ed276804ce7ea9e82d92bee293";

    # 0.10.0 release
    shellcheck-nixpkgs.url = "github:NixOS/nixpkgs/4ae2e647537bcdbb82265469442713d066675275";
  };

  outputs = {
    self,
    flake-utils,
    go-nixpkgs,
    sqlite-nixpkgs,
    nodejs-nixpkgs,
    shellcheck-nixpkgs,
  } @ inputs:
    flake-utils.lib.eachDefaultSystem (system: let
      gopkg = go-nixpkgs.legacyPackages.${system};
      go = gopkg.go_1_23;
      sqlite = sqlite-nixpkgs.legacyPackages.${system}.sqlite;
      nodejs = nodejs-nixpkgs.legacyPackages.${system}.nodejs_20;
      shellcheck = shellcheck-nixpkgs.legacyPackages.${system}.shellcheck;
    in {
      devShells.default =
        go-nixpkgs.legacyPackages.${system}.mkShell.override
        {
          stdenv = go-nixpkgs.legacyPackages.${system}.pkgsStatic.stdenv;
        }
        {
          packages = [
            gopkg.gotools
            gopkg.gopls
            gopkg.go-outline
            gopkg.gopkgs
            gopkg.gocode-gomod
            gopkg.godef
            gopkg.golint
            go
            sqlite
            nodejs
            shellcheck
          ];

          shellHook = ''
            export GOROOT="${go}/share/go"

            echo "shellcheck" "$(shellcheck --version | grep '^version:')"
            echo "node" "$(node --version)"
            echo "npm" "$(npm --version)"
            echo "sqlite" "$(sqlite3 --version | cut -d ' ' -f 1-2)"
            go version
          '';
        };

      formatter = gopkg.alejandra;
    });
}
