#!/usr/bin/env python3
import os
import zipfile

content_types_xml = """<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="json" ContentType="application/json" />
  <Default Extension="js" ContentType="application/javascript" />
  <Default Extension="ts" ContentType="text/plain" />
  <Default Extension="map" ContentType="application/json" />
  <Default Extension="md" ContentType="text/markdown" />
  <Default Extension="vsixmanifest" ContentType="text/xml" />
</Types>
"""

vsix_manifest = """<?xml version="1.0" encoding="utf-8"?>
<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011" xmlns:d="http://schemas.microsoft.com/developer/vsx-schema-design/2011">
  <Metadata>
    <Identity Id="cosm-vscode" Version="0.1.0" Publisher="cosmscm" Language="en-US" />
    <DisplayName>Cosm AST SCM &amp; Polyglot Architecture</DisplayName>
    <Description xml:space="preserve">AI-Native Polyglot AST Source Control, Micro-Universes, and 3-Tier Topology for VS Code &amp; Antigravity IDE</Description>
    <Tags>ast,scm,polyglot,merkle,micro-universe,lineage,topocosm</Tags>
    <Categories>Source Control,Programming Languages,Visualization,Other</Categories>
    <GalleryFlags>Public</GalleryFlags>
    <Properties>
      <Property Id="Microsoft.VisualStudio.Code.Engine" Value="^1.85.0" />
      <Property Id="Microsoft.VisualStudio.Code.ExtensionDependencies" Value="" />
      <Property Id="Microsoft.VisualStudio.Code.ExtensionPack" Value="" />
      <Property Id="Microsoft.VisualStudio.Code.ExtensionKind" Value="workspace" />
      <Property Id="Microsoft.VisualStudio.Code.LocalizedLanguages" Value="" />
    </Properties>
  </Metadata>
  <Installation>
    <InstallationTarget Id="Microsoft.VisualStudio.Code" />
  </Installation>
  <Dependencies />
  <Assets>
    <Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" Addressable="true" />
    <Asset Type="Microsoft.VisualStudio.Services.Content.Details" Path="extension/README.md" Addressable="true" />
  </Assets>
</PackageManifest>
"""

def package():
    base_dir = os.path.dirname(os.path.abspath(__file__))
    vsix_path = os.path.join(base_dir, "cosm-vscode-0.1.0.vsix")
    
    print(f"📦 Packaging Cosm VS Code Extension into {vsix_path}...")
    with zipfile.ZipFile(vsix_path, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("[Content_Types].xml", content_types_xml)
        z.writestr("extension.vsixmanifest", vsix_manifest)
        
        pkg_json_path = os.path.join(base_dir, "package.json")
        z.write(pkg_json_path, "extension/package.json")
        
        readme_path = os.path.join(base_dir, "README.md")
        z.write(readme_path, "extension/README.md")
        
        out_dir = os.path.join(base_dir, "out")
        count = 0
        for root, _, files in os.walk(out_dir):
            for file in sorted(files):
                full_path = os.path.join(root, file)
                rel_path = os.path.relpath(full_path, base_dir)
                arc_name = os.path.join("extension", rel_path)
                z.write(full_path, arc_name)
                count += 1
                
    file_size = os.path.getsize(vsix_path)
    print(f"✓ Successfully created {vsix_path} ({file_size} bytes, {count} compiled files included)")

if __name__ == "__main__":
    package()
