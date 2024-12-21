{
  description = "A lightweight RSS feed aggregator and reader";

  inputs = {
    flake-utils.url = "github:numtide/flake-utils";
    # 1.23.2 release
    go-nixpkgs.url = "github:NixOS/nixpkgs/4ae2e647537bcdbb82265469442713d066675275";

    # 3.44.2 release
    sqlite-nixpkgs.url = "github:NixOS/nixpkgs/5ad9903c16126a7d949101687af0aa589b1d7d3d";

    # 22.10.- release
    nodejs-nixpkgs.url = "github:NixOS/nixpkgs/566e53c2ad750c84f6d31f9ccb9d00f823165550";

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
      pkgsMusl = go-nixpkgs.legacyPackages.${system}.pkgsMusl;
      sqlite = pkgsMusl.sqlite;
      nodejs = nodejs-nixpkgs.legacyPackages.${system}.nodejs_22;
      shellcheck = shellcheck-nixpkgs.legacyPackages.${system}.shellcheck;
      mockgen = gopkg.mockgen;
    in {
      devShells.default =
        go-nixpkgs.legacyPackages.${system}.mkShell.override
        {
          stdenv = pkgsMusl.stdenv;
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
            mockgen
            pkgsMusl.musl
            pkgsMusl.gcc
          ];
          shellHook = ''
            export GOROOT="${go}/share/go"
            export CGO_ENABLED=1
            export CC="${pkgsMusl.gcc}/bin/gcc"

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
