package compose

var validLockYml = `
hash: c200cfea09886bf0b919c2067c231089e5f77ce5d5cceb119fcd1a4438d02a12
packages:
    - name: plasma-dependency-example
      source:
        type: git
        url: https://github.com/iignatevich/plasma-dependency-example.git
        ref: 0.0.1
    - name: plasma-example-http
      source:
        type: http
        url: https://github.com/iignatevich/plasma-example-http/archive/refs/tags/0.0.1.tar.gz
    - name: plasma-example
      source:
        type: git
        url: https://github.com/iignatevich/plasma-example.git
        ref: 0.0.7
      dependencies:
        - plasma-dependency-example
        - plasma-example-http
`
var invalidLockYml = `
hash: c200cfea09886bf0b919c2067c231089e5f77ce5d5cceb119fcd1a4438d02a12
packages:
    - name: plasma-dependency-example
      source:
        type: git
        url: https://github.com/iignatevich/plasma-dependency-example.git
        ref: 0.0.1
    - name: plasma-example-http
      source:
      -  type: http
      -  url: https://github.com/iignatevich/plasma-example-http/archive/refs/tags/0.0.1.tar.gz
    - name: plasma-example
      source:
        type: git
        url: https://github.com/iignatevich/plasma-example.git
        ref: 0.0.7
      dependencies:
        - plasma-dependency-example
        - plasma-example-http
`
