{
  description = "Obsrvr Flow Plugin: Soroswap Router Processor";

  nixConfig = {
    allow-dirty = true;
  };

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages = {
          default = pkgs.buildGoModule {
            pname = "flow-processor-soroswap-router";
            version = "1.0.0";
            src = ./.;
            
            # Skip vendoring check since we're using vendored dependencies
            vendorHash = null;
            
            # Disable hardening which is required for Go plugins
            hardeningDisable = [ "all" ];
            
            # Configure build environment for plugin compilation 
            preBuild = ''
              export CGO_ENABLED=1
            '';
            
            # Build as a shared library/plugin
            buildPhase = ''
              runHook preBuild
              go build -buildmode=plugin -o flow-processor-soroswap-router.so .
              runHook postBuild
            '';

            # Custom install phase for the plugin
            installPhase = ''
              runHook preInstall
              mkdir -p $out/lib
              cp flow-processor-soroswap-router.so $out/lib/
              # Also install a copy of go.mod for future reference
              mkdir -p $out/share
              cp go.mod $out/share/
              if [ -f go.sum ]; then
                cp go.sum $out/share/
              fi
              runHook postInstall
            '';
            
            # Add dependencies needed for the build
            nativeBuildInputs = [ pkgs.pkg-config ];
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [ 
            # Using Go 1.23 to match your go.mod requirements
            go_1_23
            pkg-config
            git
            gopls
            delve
          ];
          
          # Shell setup for development environment
          shellHook = ''
            # Enable CGO which is required for plugin mode
            export CGO_ENABLED=1
            
            # Helper to vendor dependencies - greatly improves build reliability
            if [ ! -d vendor ]; then
              echo "Vendoring dependencies..."
              go mod tidy
              go mod vendor
            fi
            
            echo "Development environment ready!"
            echo "To build the plugin manually: go build -buildmode=plugin -o flow-processor-soroswap-router.so ."
          '';
        };
      }
    );
} 