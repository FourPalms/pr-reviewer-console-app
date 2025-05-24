---
trigger: always_on
---

## Project Context
- This is a Golang console application that interacts with OpenAI's API
- The project follows Ben Johnson's package layout approach, organizing code by domain rather than by technical function
- The application supports both command-line and interactive modes for sending prompts to OpenAI

## Code Style and Conventions
- Follow idiomatic Go coding practices
- Use meaningful variable and function names
- Implement proper error handling with descriptive error messages
- Maintain clean separation of concerns between packages
- Document public functions and types with proper Go comments
- Keep functions focused and small (under 50 lines where possible)
- Use consistent formatting (run `go fmt` before commits)

## Testing Guidelines
- Write unit tests for all new functionality
- Aim for high test coverage, especially for business logic
- Use table-driven tests where appropriate
- Mock external dependencies in tests

## Dev notes
- When doing active dev, tend to use go run vs building an executable file.