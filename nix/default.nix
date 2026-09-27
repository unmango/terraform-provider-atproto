{
  buildGoApplication,
  lib,
  ginkgo,
  go_1_27,
  version,
}:
buildGoApplication {
  go = go_1_27;
  pname = "terraform-provider-atproto";
  inherit version;

  src = lib.cleanSource ../.;
  modules = ./gomod2nix.toml;

  nativeCheckInputs = [ ginkgo ];

  checkPhase = ''
    ginkgo run ./...
  '';
}
