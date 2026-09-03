{
  description = "chatot — GTK4 WhatsApp client (whatsmeow, in-process)";

  inputs = {
    devenv.url = "github:cachix/devenv";

    nixpkgs.url = "github:cachix/devenv-nixpkgs/rolling";
    nixpkgs-unstable.url = "github:nixos/nixpkgs/nixos-unstable";

    flake-utils.url = "github:numtide/flake-utils";

    devkit.url = "github:sezaru/nix-devkit";
  };

  nixConfig = {
    extra-trusted-public-keys = "devenv.cachix.org-1:w1cLUi8dv3hnoSPGAuibQv+f9TZLr6cv/Hm9XgU50cw=";
    extra-substituters = "https://devenv.cachix.org";
  };

  outputs = {
    devenv,
    nixpkgs,
    flake-utils,
    ...
  } @ inputs:
    flake-utils.lib.eachDefaultSystem (system: let
      pkgs = import nixpkgs {
        inherit system;
        config.allowUnfree = true;
      };
    in {
      # `nix build` / `nix profile install .` — the desktop app (see
      # .nix/package.nix for what the wrapper provides).
      packages = {
        chatot = pkgs.callPackage ./.nix/package.nix {};
        default = inputs.self.packages.${system}.chatot;
      };

      apps.default = {
        type = "app";
        program = "${inputs.self.packages.${system}.chatot}/bin/chatot";
      };

      devShells.default = devenv.lib.mkShell {
        inherit inputs pkgs;
        modules = [
          ./.nix/devenv.nix
        ];
      };
    });
}
