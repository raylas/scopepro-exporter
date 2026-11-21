{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      ...
    }:
    flake-utils.lib.eachSystem
      [
        "aarch64-linux"
        "x86_64-linux"
        "aarch64-darwin"
      ]
      (
        system:
        let
          pkgs = import nixpkgs {
            inherit system;
          };
          version = self.rev or "dev";
        in
        {
          packages =
            {
              default = pkgs.buildGoModule {
                pname = "scopepro-exporter";
                inherit version;
                src = ./.;
                vendorHash = "sha256-8zjE8+1no3hvWd4rygoSmChYjQWUXKPb7q8zsgXoOV0=";
                env.CGO_ENABLED = 0;
                ldflags = [
                  "-s"
                  "-w"
                  "-X main.version=${version}"
                ];
                subPackages = [ "cmd/scopepro-exporter" ];
              };
            }
            // pkgs.lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
              image = pkgs.dockerTools.buildLayeredImage {
                name = "scopepro-exporter";
                tag = version;
                contents = [ self.packages.${system}.default ];
                config = {
                  Entrypoint = [ "/bin/scopepro-exporter" ];
                };
              };
            };

          devShell = pkgs.mkShellNoCC {
            buildInputs = with pkgs; [
              go
              gopls
              golangci-lint-langserver
            ];

            CGO_ENABLED = "0";
          };
        }
      );
}
