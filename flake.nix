{
  description = "GoVista — Wallpaper Browser";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages.default = pkgs.buildGoModule {
          pname = "govista";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-Ohc9irHFLbhYcDS+Gx5bvgqNrW+1uJvcsg9lz/01aD4=";
          nativeBuildInputs = [ pkgs.pkg-config ];
          buildInputs = with pkgs; [
            libGL
            wayland
            wayland-protocols
            libxkbcommon
            libx11
            libxcursor
            libxfixes
            libxcb
            vulkan-loader
            vulkan-headers
          ];
          env.CGO_ENABLED = "1";
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            pkg-config
            libGL
            wayland
            wayland-protocols
            libxkbcommon
            libx11
            libxcursor
            libxfixes
            libxcb
            vulkan-loader
            vulkan-headers
          ];

          shellHook = ''
            export CGO_ENABLED=1
            export LD_LIBRARY_PATH="${pkgs.lib.makeLibraryPath (with pkgs; [
              libGL
              wayland
              libxkbcommon
              libx11
              vulkan-loader
            ])}:$LD_LIBRARY_PATH"

            echo "GoVista dev shell ready."
            echo "Run the following once to fetch dependencies:"
            echo "  go get gioui.org@latest"
            echo "  go get gioui.org/x@latest"
            echo "  go get github.com/davenicholson-xyz/go-wallhaven@latest"
            echo "  go get github.com/davenicholson-xyz/go-setwallpaper@latest"
            echo "  go mod tidy"
          '';
        };
      }
    );
}
