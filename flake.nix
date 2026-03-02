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

          vendorHash = "sha256-/Et/G1QcXxFckjtMmGDlqYqHHPIdQg+6Bz0bZETA5oA=";

          subPackages = [ "." ];

          ldflags = [ "-s" "-w" ];

          postInstall = ''
            ln -sf $out/bin/nostr-dump-cli $out/bin/nostr-dump
          '';

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
