package compose

var validLockYml = `
hash: c200cfea09886bf0b919c2067c231089e5f77ce5d5cceb119fcd1a4438d02a12
packages:
    - name: compose-dependency-example
      source:
        type: git
        url: https://github.com/iignatevich/compose-dependency-example.git
        ref: 0.0.1
    - name: compose-example-http
      source:
        type: http
        url: https://github.com/iignatevich/compose-example-http/archive/refs/tags/0.0.1.tar.gz
    - name: compose-example
      source:
        type: git
        url: https://github.com/iignatevich/compose-example.git
        ref: 0.0.7
      dependencies:
        - compose-dependency-example
        - compose-example-http
`
var invalidLockYml = `
hash: c200cfea09886bf0b919c2067c231089e5f77ce5d5cceb119fcd1a4438d02a12
packages:
    - name: compose-dependency-example
      source:
        type: git
        url: https://github.com/iignatevich/compose-dependency-example.git
        ref: 0.0.1
    - name: compose-example-http
      source:
      -  type: http
      -  url: https://github.com/iignatevich/compose-example-http/archive/refs/tags/0.0.1.tar.gz
    - name: compose-example
      source:
        type: git
        url: https://github.com/iignatevich/compose-example.git
        ref: 0.0.7
      dependencies:
        - compose-dependency-example
        - compose-example-http
`
