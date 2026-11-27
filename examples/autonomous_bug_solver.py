#!/usr/bin/env python3
"""
Autonomous Bug Solver - Advanced Multi-Agent Example

This example demonstrates building an autonomous bug-solving system using
Hanzo Agent SDK's network and orchestration features. Inspired by AgentKit's
guided tour, this shows how multiple specialized agents can work together
to understand, diagnose, and fix bugs in code.
"""

import asyncio
import json
import os
from typing import Any, Dict, List

from openai import AsyncOpenAI

from agents import (
    Agent,
    Model,
    ModelProvider,
    OpenAIChatCompletionsModel,
    function_tool,
)
from agents.memory import VectorMemoryStore
from agents.network import (
    RuleBasedRouter,
    RoutingStrategy,
    SemanticRouter,
    create_network,
)
from agents.orchestration import Step, Workflow, WorkflowExecutor
from agents.state import InMemoryStateStore

# Configure Hanzo backend
HANZO_ROUTER_URL = os.getenv("HANZO_ROUTER_URL", "http://localhost:4000/v1")
HANZO_API_KEY = os.getenv("HANZO_API_KEY", "sk-1234")


class HanzoModelProvider(ModelProvider):
    """Custom model provider routing requests through Hanzo Router."""

    def __init__(self, base_url: str = HANZO_ROUTER_URL, api_key: str = HANZO_API_KEY):
        self.client = AsyncOpenAI(base_url=base_url, api_key=api_key)

    def get_model(self, model_name: str | None) -> Model:
        return OpenAIChatCompletionsModel(
            model=model_name or "gpt-4",
            openai_client=self.client,
        )


# Initialize model provider
model_provider = HanzoModelProvider(HANZO_ROUTER_URL, HANZO_API_KEY)


# ==================== Tools ====================

@function_tool
async def read_file(file_path: str) -> str:
    """Read a file from the filesystem."""
    try:
        with open(file_path, 'r') as f:
            return f.read()
    except Exception as e:
        return f"Error reading file: {e}"


@function_tool
async def write_file(file_path: str, content: str) -> str:
    """Write content to a file."""
    try:
        # Create backup first
        if os.path.exists(file_path):
            backup_path = f"{file_path}.backup"
            with open(file_path, 'r') as f:
                backup_content = f.read()
            with open(backup_path, 'w') as f:
                f.write(backup_content)
        
        with open(file_path, 'w') as f:
            f.write(content)
        return f"File written successfully to {file_path}"
    except Exception as e:
        return f"Error writing file: {e}"


@function_tool
async def run_tests(test_command: str = "pytest") -> Dict[str, Any]:
    """Run tests and return results."""
    import subprocess
    try:
        result = subprocess.run(
            test_command.split(),
            capture_output=True,
            text=True,
            timeout=30
        )
        return {
            "success": result.returncode == 0,
            "stdout": result.stdout,
            "stderr": result.stderr,
            "return_code": result.returncode
        }
    except Exception as e:
        return {
            "success": False,
            "error": str(e)
        }


@function_tool
async def analyze_stack_trace(error_text: str) -> Dict[str, Any]:
    """Analyze a stack trace to identify the error location and type."""
    lines = error_text.split('\n')
    
    # Simple parser - in production use proper parsing
    error_info = {
        "error_type": None,
        "error_message": None,
        "file_path": None,
        "line_number": None,
        "function_name": None
    }
    
    for i, line in enumerate(lines):
        if "File " in line and "line " in line:
            # Extract file path and line number
            parts = line.split('"')
            if len(parts) >= 2:
                error_info["file_path"] = parts[1]
                
            if "line " in line:
                line_parts = line.split("line ")
                if len(line_parts) >= 2:
                    error_info["line_number"] = line_parts[1].split(",")[0].strip()
                    
        if i < len(lines) - 1 and not lines[i+1].startswith(" "):
            # This might be the error type and message
            if "Error" in line or "Exception" in line:
                parts = line.split(":", 1)
                if len(parts) >= 2:
                    error_info["error_type"] = parts[0].strip()
                    error_info["error_message"] = parts[1].strip()
    
    return error_info


@function_tool
async def search_codebase(pattern: str, file_types: List[str] = None) -> List[Dict[str, Any]]:
    """Search codebase for specific patterns."""
    import os
    import re
    
    if file_types is None:
        file_types = ['.py', '.js', '.ts', '.java', '.go']
    
    results = []
    
    for root, dirs, files in os.walk('.'):
        # Skip hidden directories and common ignore patterns
        dirs[:] = [d for d in dirs if not d.startswith('.') and d not in ['node_modules', '__pycache__']]
        
        for file in files:
            if any(file.endswith(ft) for ft in file_types):
                file_path = os.path.join(root, file)
                try:
                    with open(file_path, 'r') as f:
                        content = f.read()
                        matches = list(re.finditer(pattern, content, re.MULTILINE | re.IGNORECASE))
                        if matches:
                            for match in matches:
                                line_num = content[:match.start()].count('\n') + 1
                                results.append({
                                    "file": file_path,
                                    "line": line_num,
                                    "match": match.group(),
                                    "context": content.split('\n')[line_num-1]
                                })
                except:
                    pass
    
    return results


@function_tool
async def fix_and_test(file_path: str, content: str, test_command: str = "pytest") -> Dict[str, Any]:
    """Apply a fix to a file and immediately run tests."""
    write_msg = await write_file.on_invoke_tool(
        None,
        json.dumps({"file_path": file_path, "content": content}),
    )
    test_result = await run_tests.on_invoke_tool(
        None,
        json.dumps({"test_command": test_command}),
    )
    return {"write_result": write_msg, "test_result": test_result}


# ==================== Agents ====================

# Bug Analyzer Agent
bug_analyzer = Agent(
    name="BugAnalyzer",
    instructions="""You are an expert at analyzing bugs and errors in code.
    Your role is to:
    1. Analyze error messages and stack traces
    2. Identify the root cause of issues
    3. Determine the scope and impact of bugs
    4. Suggest investigation strategies
    
    Be thorough and systematic in your analysis.""",
    tools=[analyze_stack_trace, read_file, search_codebase],
    model=model_provider.get_model("gpt-4"),
)

# Code Reader Agent
code_reader = Agent(
    name="CodeReader",
    instructions="""You are an expert at reading and understanding code.
    Your role is to:
    1. Read relevant code files
    2. Understand code structure and dependencies
    3. Identify potential problem areas
    4. Explain code functionality clearly
    
    Focus on understanding the code's intent and implementation.""",
    tools=[read_file, search_codebase],
    model=model_provider.get_model("gpt-3.5-turbo"),  # Faster for simple reading tasks
)

# Solution Designer Agent
solution_designer = Agent(
    name="SolutionDesigner",
    instructions="""You are an expert software architect and problem solver.
    Your role is to:
    1. Design solutions for identified bugs
    2. Consider multiple approaches
    3. Evaluate trade-offs
    4. Propose the best fix strategy
    
    Think about edge cases, performance, and maintainability.""",
    tools=[read_file],
    model=model_provider.get_model("gpt-4"),
)

# Code Fixer Agent
code_fixer = Agent(
    name="CodeFixer",
    instructions="""You are an expert at implementing bug fixes.
    Your role is to:
    1. Implement the proposed solutions
    2. Write clean, maintainable code
    3. Add appropriate error handling
    4. Update related code if needed
    
    Always test your fixes and handle edge cases.""",
    tools=[read_file, write_file, fix_and_test],
    model=model_provider.get_model("gpt-4"),
)

# Test Writer Agent
test_writer = Agent(
    name="TestWriter",
    instructions="""You are an expert at writing comprehensive tests.
    Your role is to:
    1. Write tests that cover the bug fix
    2. Add edge case tests
    3. Ensure regression prevention
    4. Follow testing best practices
    
    Write clear, maintainable tests with good coverage.""",
    tools=[read_file, write_file, run_tests],
    model=model_provider.get_model("gpt-3.5-turbo"),
)

# Quality Reviewer Agent
quality_reviewer = Agent(
    name="QualityReviewer",
    instructions="""You are a senior engineer reviewing code changes.
    Your role is to:
    1. Review the implemented fix
    2. Check for potential issues
    3. Verify tests are adequate
    4. Ensure code quality standards
    
    Be constructive but thorough in your review.""",
    tools=[read_file, run_tests],
    model=model_provider.get_model("gpt-4"),
)


# ==================== Network Setup ====================

# Create memory stores for agents
memory_store = VectorMemoryStore(collection_name="bug_solver_memory")

# Set up routers
semantic_router = SemanticRouter()

rule_router = RuleBasedRouter()
rule_router.add_rule(r".*error.*|.*exception.*|.*trace.*", "BugAnalyzer")
rule_router.add_rule(r".*read.*|.*understand.*|.*explain.*", "CodeReader")
rule_router.add_rule(r".*design.*|.*approach.*|.*solution.*", "SolutionDesigner")
rule_router.add_rule(r".*fix.*|.*implement.*|.*patch.*", "CodeFixer")
rule_router.add_rule(r".*test.*|.*coverage.*", "TestWriter")
rule_router.add_rule(r".*review.*|.*quality.*|.*check.*", "QualityReviewer")

# Create the network using the rule-based router
bug_solver_network = create_network(
    agents=[
        bug_analyzer,
        code_reader,
        solution_designer,
        code_fixer,
        test_writer,
        quality_reviewer,
    ],
    router=rule_router,
    state_store=InMemoryStateStore(),
    name="Autonomous Bug Solver Network",
)


# ==================== Workflow ====================

# Define the bug-solving workflow using Workflow + Step
bug_solving_workflow = Workflow(
    name="Autonomous Bug Solver",
    description="Analyze, fix, test, and review software bugs",
)
analyze_step = Step.agent("BugAnalyzer", "Analyze the error: {error_message}")
design_step = Step.agent("SolutionDesigner", "Design a fix based on the analysis")
design_step.depends_on = [analyze_step.id]
fix_step = Step.agent("CodeFixer", "Implement the proposed solution")
fix_step.depends_on = [design_step.id]
test_step = Step.agent("TestWriter", "Write regression tests for the fix")
test_step.depends_on = [fix_step.id]
review_step = Step.agent("QualityReviewer", "Review the complete fix and tests")
review_step.depends_on = [test_step.id]

for _step in (analyze_step, design_step, fix_step, test_step, review_step):
    bug_solving_workflow.add_step(_step)
bug_solving_workflow.entry_point = analyze_step.id


# ==================== Main Example ====================

async def solve_bug(error_message: str, context: Dict[str, Any] | None = None):
    """Autonomously solve a bug given an error message."""
    print("Autonomous Bug Solver Started")
    print("=" * 60)
    print(f"Error: {error_message}")
    print("=" * 60)

    executor = WorkflowExecutor(bug_solving_workflow, bug_solver_network)
    initial_state = dict(context or {})
    initial_state["error_message"] = error_message
    result = await executor.execute(
        f"Analyze and fix the following error:\n{error_message}",
        context=initial_state,
    )
    print(f"Workflow success: {result.success}")
    return result


# ==================== Example Usage ====================

async def main():
    """Run example bug-solving scenarios."""
    
    # Example 1: Simple syntax error
    print("\n" + "="*80)
    print("Example 1: Solving a Simple Syntax Error")
    print("="*80)
    
    error1 = """
    Traceback (most recent call last):
      File "app.py", line 42, in process_data
        result = calculate_total(items)
      File "utils/calculator.py", line 15, in calculate_total
        total += item.price * item.quantty
    AttributeError: 'Item' object has no attribute 'quantty'
    """
    
    await solve_bug(error1)
    
    # Example 2: Complex logic error
    print("\n" + "="*80)
    print("Example 2: Solving a Complex Logic Error")
    print("="*80)
    
    error2 = """
    Test test_payment_processing failed:
    AssertionError: Payment total mismatch
    Expected: 150.00
    Actual: 135.00
    
    This happens when applying multiple discount codes to an order.
    The discount calculation in checkout.py might not be handling
    overlapping discounts correctly.
    """
    
    await solve_bug(error2, context={
        "test_file": "tests/test_checkout.py",
        "suspected_files": ["checkout.py", "models/discount.py"]
    })
    
    # Show network statistics
    print("\n" + "="*80)
    print("Network Statistics")
    print("="*80)
    
    stats = bug_solver_network.get_statistics()
    for agent_name, agent_stats in stats.items():
        print(f"\n{agent_name}:")
        print(f"  Calls: {agent_stats['total_calls']}")
        print(f"  Avg Duration: {agent_stats['avg_duration']:.2f}ms")
        print(f"  Success Rate: {agent_stats['success_rate']:.1%}")


if __name__ == "__main__":
    asyncio.run(main())