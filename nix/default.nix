{
  buildGoApplication,
  lib,
  ginkgo,
  go_1_27,
  opentofu,
  version,
}:
buildGoApplication {
  go = go_1_27;
  pname = "terraform-provider-atproto";
  inherit version;

  src = lib.cleanSource ../.;
  modules = ./gomod2nix.toml;

  nativeCheckInputs = [
    ginkgo
    opentofu
  ];

  # The specs drive OpenTofu against a local fake PDS, so they need no network.
  checkPhase = ''
    export TF_ACC_TERRAFORM_PATH=${lib.getExe opentofu}
    export TF_ACC_PROVIDER_NAMESPACE=hashicorp
    export TF_ACC_PROVIDER_HOST=registry.opentofu.org
    ginkgo run ./...
  '';
}
