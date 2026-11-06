# AGENTS.md

## Purpose

This file defines general rules for organizing, designing, documenting, and verifying Go projects.

The main priorities are:

1. Simplicity.
2. Clarity.
3. Verifiability.
4. Predictable behavior.
5. A minimal number of abstractions.

Architectural perfection is not a goal in itself. A simple and somewhat less universal solution is preferable to a complex architecture whose purpose is difficult to understand and verify.

## General Go Rules

- Use the Go version specified in `go.mod` or in the project requirements.
- Use the standard library unless the project explicitly requires otherwise.
- Format source files with `gofmt`.
- Use ordinary, well-established Go features and patterns.
- Do not use `unsafe`, reflection, untyped containers, or other complex mechanisms without an explicit need.
- Do not add dependencies, frameworks, or generic infrastructure layers only for future possibilities.
- Do not add mutable global state without a specific and justified requirement.
- Do not add code that is not needed for the current task.

## Package Organization

- Each package should have one clear area of responsibility.
- Dependencies between packages should be directed and obvious.
- Do not allow cyclic imports.
- Do not create intermediate packages, wrappers, or forwarding APIs without a real need.
- Do not duplicate the same logic across several packages.
- A package should not depend on implementation details it does not need.

## File Organization

Files should be small and contain logically related code.

- The normal file limit is 300 lines, including comments and blank lines.
- A file may exceed 300 lines if it contains one genuinely large type with many closely related methods.
- Exceeding the limit is not a reason to split code mechanically. If splitting makes the code harder to understand, do not do it.
- Do not combine unrelated types, functions, and constants in one file.

### Types and Methods

- The main type and its methods should normally be placed in a separate file.
- Methods belonging to one type should be kept together and should not be mixed with the implementation of other large types.
- Small, closely related types may be grouped in one file.
- Types belonging to different subsystems or having different purposes should be placed in different files.
- A file containing a type should not become a collection of unrelated helper functions.
- If a type becomes large, its methods may be split into several files by purpose while keeping one clear type in one package.

### Functions

- Functions without a common owner should be grouped by purpose.
- For example, parsing, validation, formatting, and conversion functions should not be mixed in one file without a reason.
- Do not create a separate file for every small function.
- Do not combine functions merely because they use the same types.
- A file name should help explain what kind of code it contains.

### File Names

- File names should reflect their purpose.
- Do not use vague names such as `misc.go`, `common.go`, or `helpers.go` when a file contains unrelated logic.
- Helper functions should be placed in a file matching their actual area of use, rather than automatically being placed in a common `helpers.go` file.

## Type and API Design

- Export only the types, functions, methods, and fields that are part of the required API.
- Struct fields should be private by default.
- Prefer concrete types over interfaces.
- Create an interface only when there is a real substitution boundary, multiple implementations, or a need to isolate a dependency in tests.
- Do not create interfaces in advance for possible future use.
- Do not use pointers, caches, or accessors when they hide mutable state without a clear benefit.
- Ownership of mutable data must be clear.
- Do not return internal mutable slices or maps unless necessary. Make a copy when needed.
- Use slices to preserve order instead of relying on map iteration order.
- Use a separate named type only when it clarifies the meaning of a value or protects an important contract.
- Do not add generic abstraction layers, factories, or dispatchers without a concrete task that requires them.
- The public API should be as small and simple as possible.

## Functions and Control Flow

- Each function should perform one clear task.
- A function should have explicit parameters and results.
- A large function should be checked for mixed responsibilities.
- Split a function when the extracted part has an independent purpose, a meaningful name, and testable behavior.
- Do not split linear code into many meaningless functions merely to reduce the number of lines.
- Prefer early returns and shallow nesting.
- Do not hide important actions behind a chain of insignificant wrappers.

## Naming and Readability

- Follow standard Go naming conventions.
- Use short but understandable names.
- Use common abbreviations and initialisms according to Go conventions.
- Do not repeat the package name in an exported identifier unless necessary.
- Do not use vague names such as `data`, `value`, `item`, `process`, or `handle` when a more precise name is possible.
- Group constants by meaning.
- Prefer a readable expression over a short but obscure one.

## Errors and State

- Return errors in the usual Go manner.
- Add context with `fmt.Errorf` and `%w` when the original error must be preserved.
- Do not compare errors by their text when an error type or sentinel error can be used.
- Do not use `panic` to handle ordinary input, file errors, parsing errors, or other expected situations.
- Do not use panic recovery as a normal error-handling mechanism.
- Keep state as close as possible to the operation that uses it.
- Do not reuse mutable state between independent operations without an explicit need.
- Error behavior must be predictable and testable.

## Comments and Documentation

- Comments should explain purpose and important constraints, not restate every line of code.
- Write Go documentation comments in Russian unless the project specifies otherwise.
- A documentation comment for an identifier must begin with its exact name.
- A comment should state, where relevant:
  - purpose;
  - important constraints;
  - units of measurement;
  - side effects;
  - data ownership;
  - error behavior.
- For an ambiguous Boolean result, explain the meaning of both `true` and `false`.
- If a range has an exclusive right boundary, state this explicitly.
- Do not leave `TODO` markers for requirements belonging to the current task.

## Testing

- Test observable behavior rather than the internal structure of the code.
- Tests should be independent of execution order and external state.
- Use table-driven tests for sets of similar cases.
- Test names should describe the behavior being checked.
- Test helper functions should use `t.Helper()`.
- Use `t.Run()` for individual scenarios.
- Use `t.Parallel()` only when parallel execution is safe and does not reduce readability.
- Store substantial test data in `testdata/`.
- Do not remove, weaken, or skip a valid test merely to make the implementation pass.
- Every discovered defect should be covered by a separate regression test.
- Tests should not require a network, random state, or a specific execution order unless that is part of the behavior being tested.

## Verification

Before completing the work:

- format changed Go files with `gofmt`;
- run the project's tests;
- run static checks if they are provided by the project;
- verify that every new public object is actually needed;
- verify that there are no cyclic dependencies;
- verify error handling;
- verify that the file structure remains clear;
- ensure that comments match the actual behavior of the code.

The exact verification commands are determined by the project and should be specified in its technical requirements or `README`. Do not require commands that are unrelated to the project.
