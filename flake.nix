{
  description = "nostr-dump-cli: dump nostr events by npub with relay pagination";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "nostr-dump-cli";
          version = "0.1.0";

          src = ./.;

          vendorHash = "sha256-rip6+uMDo4h3DfUQ4fYn+nPV9B8XxD6fexkCT2qtqLQ=";

          subPackages = [ "." ];

          ldflags = [ "-s" "-w" ];

          meta = with pkgs.lib; {
            description = "CLI to fetch nostr events for an npub with pagination across relays";
            license = licenses.mit;
            mainProgram = "nostr-dump";
            platforms = platforms.unix;
          };
        };

        apps.default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/nostr-dump";
        };

        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.gotools
          ];
        };
      });
}
