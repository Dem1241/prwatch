{
  description = "see which of your open github PRs need you";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAll (pkgs: {
        default = pkgs.buildGoModule {
          pname = "prwatch";
          version = "0.1.0";
          src = self;
          vendorHash = null;
          env.CGO_ENABLED = 0;
          meta.mainProgram = "prwatch";
        };
      });

      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.gh ];
          CGO_ENABLED = "0";
        };
      });
    };
}
