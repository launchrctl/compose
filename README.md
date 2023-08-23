## Composition Tool Specification
The composition tool is a command-line tool that helps developers manage
dependencies for their projects. It allows developers to specify the dependencies for
a project in a "compose.yaml" file, and then fetches and installs those dependencies
in a structured and organized way.

The tool works by recursively fetching and processing the "compose.yaml" files for each package
and its dependencies, and then merging the resulting filesystems into a single filesystem.

### CLI
The composition tool is invoked from the command line with the following syntax:
launchr compose [options]
Where options are:
* -w, --working-dir : The directory where temporary files should be stored during the
  composition process. Default is the .compose/packages
* -s, --skip-not-versioned : Skip not versioned files from source directory (git only)

example usage - `launchr compose -w=./folder/something -s=1 or -s=true`

### `compose.yaml` File Format
The "compose.yaml" file is a text file that specifies the dependencies for a package, along with any necessary metadata and sources for those dependencies.
The file format includes the following elements:
- name: The name of the package.
- version: The version number of the package.
- source: The source for the package, including the type of source (e.g., Git, HTTP), URL or file path, and other metadata as needed.
- dependencies: A list of required dependencies.

Example:

```yaml
name: myproject
version: 1.0.0
dependencies:
- name: compose-example
  source:
    type: git
    ref: master
    tag: 0.0.1
    url: https://github.com/example/compose-example.git
```

`Lock example`

```yaml
hash: c200cfea09886bf0b919c2067c231089e5f77ce5d5cceb119fcd1a4438d02a12
packages:
  - name: compose-dependency-example
    source:
      type: git
      url: https://github.com/example/compose-dependency-example.git
      ref: master
      tag: 0.0.1
  - name: compose-example-http
    source:
      type: http
      url: https://github.com/example/compose-example-http/archive/refs/tags/0.0.1.tar.gz
  - name: compose-example
    source:
      type: git
      url: https://github.com/example/compose-example.git
      ref: master
      tag: 0.0.7
    dependencies:
      - compose-dependency-example
      - compose-example-http
```


### Fetching and Installing Dependencies
The composition tool fetches and installs dependencies for a package by recursively processing the "compose.yaml" files for each package and its dependencies. The tool follows these general steps:

1. Fetch the package source code from the specified source location.
2. Extract the package contents to a temporary directory.
3. Process the "compose.yaml" file for the package, fetching and installing any necessary dependencies recursively.
4. Merge the package filesystem into the final platform filesystem.
5. Repeat steps 1-4 for each package and its dependencies.

During this process, the composition tool keeps track of the dependencies for each package.
