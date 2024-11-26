## Composition Tool Specification

The composition tool is a command-line tool that helps developers manage
dependencies for their projects. It allows developers to specify the dependencies for
a project in a "plasma-compose.yaml" file, and then fetches and installs those dependencies
in a structured and organized way.

The tool works by recursively fetching and processing the "plasma-compose.yaml" files for each package
and its dependencies, and then merging the resulting filesystems into a single filesystem.

### CLI

The composition tool is invoked from the command line with the following syntax:
launchr compose [options]
Where options are:

* -w, --working-dir : The directory where temporary files should be stored during the
  composition process. Default is the .compose/packages
* -s, --skip-not-versioned : Skip not versioned files from source directory (git only)
* --conflicts-verbosity: Log files conflicts in format "[current-package] - path to file > Selected
  from [domain, other package or current-package]"
* --interactive: Interactive mode allows to submit user credentials during action (default: true)

Example usage - `launchr compose -w=./folder/something -s=1 or -s=true --conflicts-verbosity`

It's important to note that: if same file is present locally and also brought by a package, default strategy is that
local file will be taken and package file
ignored. [Different strategies](https://github.com/launchrctl/compose/blob/main/example/compose.example.yaml#L18-L35)
can be difined to customize this behavior to your needs.

### `plasma-compose.yaml` File Format

The "plasma-compose.yaml" file is a text file that specifies the dependencies for a package, along with any necessary
metadata and sources for those dependencies.
The file format includes the following elements:

- name: The name of the package.
- version: The version number of the package.
- source: The source for the package, including the type of source (Git, HTTP), URL or file path, merge strategy and
  other metadata.
- dependencies: A list of required dependencies.

List of strategies:

- overwrite-local-file
- remove-extra-local-files
- ignore-extra-package-files
- filter-package-files

Example:

```yaml
name: example
dependencies:
  - name: compose-example
    source:
      type: git
      ref: master # branch or tag name
      url: https://github.com/example/compose-example.git
      strategy:
        - name: remove-extra-local-files
          path:
            - path/to/remove-extra-local-files
        - name: ignore-extra-package-files
          path:
            - library/inventories/platform_nodes/configuration/dev.yaml
            - library/inventories/platform_nodes/configuration/prod.yaml
            - library/inventories/platform_nodes/configuration/whatever.yaml
```

### Fetching and Installing Dependencies

The composition tool fetches and installs dependencies for a package by recursively processing the "plasma-compose.yaml"
files for each package and its dependencies. The tool follows these general steps:

1. Check if package exists locally and is up-to-date. If it's not, remove it from packages dir and proceed to next step.
2. Fetch the package from the specified location.
3. Extract the package contents to a packages directory.
4. Process the "plasma-compose.yaml" file for the package, fetching and installing any necessary dependencies
   recursively.
5. Merge the package filesystem into the final platform filesystem.
6. Repeat steps 1-5 for each package and its dependencies.

During this process, the composition tool keeps track of the dependencies for each package.

### Plasma-compose commands

it's possible to manipulate plasma-compose.yaml file using commands:

- plasmactl compose:add
- plasmactl compose:update
- plasmactl compose:delete

For `compose:add` and `compose:update` there are 2 ways to submit data. With or without flags.
Passing `--package` and `--url` to add command will automatically update plasma-compose file.
For update command only `--package` required to update from CLI.

For `compose:delete` it's possible to pass list of packaged to delete.

In other cases, user will be prompted to CLI form to fill necessary data of packages.

Examples of usage

```
launchr compose:add --url some-url --type http
launchr compose:add --package package-name --url some-url --ref v1.0.0
launchr compose:update --package package-name --url some-url --ref v1.0.0

launchr compose:add --package package-name --url some-url --ref v1.0.0 --strategy overwrite-local-file --strategy-path "path1|path2"
launchr compose:add --package package-name --url some-url --ref branch --strategy overwrite-local-file,remove-extra-local-files --strategy-path "path1|path2,path3|path4"
launchr compose:add --package package-name --url some-url --ref v1.0.0 --strategy overwrite-local-file --strategy-path "path1|path2" --strategy remove-extra-local-files --strategy-path "path3|path4"
```