{
  description = "A Terraform provider for AT Protocol records";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    systems.url = "github:UnstoppableMango/nix-systems";

    flake-parts = {
      url = "github:hercules-ci/flake-parts";
      inputs.nixpkgs-lib.follows = "nixpkgs";
    };

    treefmt-nix = {
      url = "github:numtide/treefmt-nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.inputs.systems.follows = "systems";
    };
  };

  outputs =
    inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = import inputs.systems;
      imports = with inputs; [
        systems.flakeModule
        treefmt-nix.flakeModule
      ];

      perSystem =
        { pkgs, system, ... }:
        let
          version = pkgs.lib.strings.trim (builtins.readFile ./VERSION);
        in
        {
          _module.args.pkgs = import inputs.nixpkgs {
            inherit system;
            overlays = with inputs; [ gomod2nix.overlays.default ];
          };

          packages.default = pkgs.callPackage ./nix { inherit version; };

          devShells.default = pkgs.mkShellNoCC {
            packages = with pkgs; [
              direnv
              go_1_27
              gomod2nix
              gopls
              gnumake
              nixfmt
              golangci-lint
              opentofu
              terraform-plugin-docs
            ];

            TF_ACC_TERRAFORM_PATH = "${pkgs.opentofu}/bin/tofu";
            TF_ACC_PROVIDER_NAMESPACE = "hashicorp";
            TF_ACC_PROVIDER_HOST = "registry.opentofu.org";
          };

          treefmt.programs = {
            actionlint.enable = true;
            gofmt.enable = true;
            mdformat.enable = true;
            nixfmt.enable = true;
            shellcheck.enable = true;
          };

          # mdformat rewrites YAML frontmatter as a heading, which breaks generated
          # docs pages and agent skills alike.
          treefmt.settings.formatter.mdformat.excludes = [
            ".github/skills/**"
            "CHANGELOG.md"
            "docs/index.md"
            "docs/data-sources/**"
            "docs/resources/**"
            "docs/guides/**"
          ];

          # Import examples are shell fragments that tfplugindocs copies into the
          # docs verbatim, so they carry no shebang.
          treefmt.settings.formatter.shellcheck.excludes = [
            "examples/**/import.sh"
          ];
        };
    };
}
