// Command features tests TypeScript language features across GOJA and Bun engines.
//
// This command runs a comprehensive test suite to verify that various TypeScript
// and JavaScript language features work correctly on both execution engines.
package main

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/koltyakov/tsgo"
)

// TestCase defines a TypeScript feature test.
type TestCase struct {
	Name     string
	Code     string
	Expected any
	Compare  func(any) bool
	Skip     string
}

// TestCategory groups related test cases.
type TestCategory struct {
	Name  string
	Tests []TestCase
}

// Test categories covering TypeScript language features
var testCategories = []TestCategory{
	{
		Name: "Primitive Types & Literals",
		Tests: []TestCase{
			{Name: "number literal", Code: `export default 42`, Expected: int64(42)},
			{Name: "float literal", Code: `export default 3.14159`, Expected: 3.14159},
			{Name: "negative number", Code: `export default -273.15`, Expected: -273.15},
			{Name: "string literal", Code: `export default "hello"`, Expected: "hello"},
			{Name: "boolean true", Code: `export default true`, Expected: true},
			{Name: "boolean false", Code: `export default false`, Expected: false},
			{Name: "null value", Code: `export default null`, Expected: nil},
			{Name: "undefined", Code: `export default undefined`, Expected: nil},
		},
	},
	{
		Name: "Arithmetic Operations",
		Tests: []TestCase{
			{Name: "addition", Code: `export default 10 + 5`, Expected: int64(15)},
			{Name: "subtraction", Code: `export default 10 - 5`, Expected: int64(5)},
			{Name: "multiplication", Code: `export default 10 * 5`, Expected: int64(50)},
			{Name: "division", Code: `export default 10 / 4`, Expected: 2.5},
			{Name: "modulo", Code: `export default 17 % 5`, Expected: int64(2)},
			{Name: "exponentiation", Code: `export default 2 ** 10`, Expected: int64(1024)},
			{Name: "increment", Code: `let x = 5; x++; export default x`, Expected: int64(6)},
			{Name: "compound add", Code: `let x = 10; x += 5; export default x`, Expected: int64(15)},
			{Name: "unary minus", Code: `const x = 5; export default -x`, Expected: int64(-5)},
		},
	},
	{
		Name: "Comparison & Logical Operators",
		Tests: []TestCase{
			{Name: "equality", Code: `export default 5 === 5`, Expected: true},
			{Name: "inequality", Code: `export default 5 !== 3`, Expected: true},
			{Name: "less than", Code: `export default 3 < 5`, Expected: true},
			{Name: "greater than", Code: `export default 5 > 3`, Expected: true},
			{Name: "logical AND", Code: `export default true && true`, Expected: true},
			{Name: "logical OR", Code: `export default false || true`, Expected: true},
			{Name: "logical NOT", Code: `export default !false`, Expected: true},
			{Name: "nullish coalescing", Code: `const x = null; export default x ?? "default"`, Expected: "default"},
			{Name: "ternary operator", Code: `export default true ? "yes" : "no"`, Expected: "yes"},
		},
	},
	{
		Name: "String Operations",
		Tests: []TestCase{
			{Name: "concatenation", Code: `export default "hello" + " " + "world"`, Expected: "hello world"},
			{Name: "template literal", Code: "const name = \"TS\"; export default `Hello ${name}!`", Expected: "Hello TS!"},
			{Name: "length", Code: `export default "hello".length`, Expected: int64(5)},
			{Name: "toUpperCase", Code: `export default "hello".toUpperCase()`, Expected: "HELLO"},
			{Name: "toLowerCase", Code: `export default "HELLO".toLowerCase()`, Expected: "hello"},
			{Name: "trim", Code: `export default "  hello  ".trim()`, Expected: "hello"},
			{Name: "split", Code: `export default "a,b,c".split(",").length`, Expected: int64(3)},
			{Name: "substring", Code: `export default "hello".substring(1, 4)`, Expected: "ell"},
			{Name: "slice", Code: `export default "hello".slice(-2)`, Expected: "lo"},
			{Name: "includes", Code: `export default "hello".includes("ell")`, Expected: true},
			{Name: "startsWith", Code: `export default "hello".startsWith("he")`, Expected: true},
			{Name: "endsWith", Code: `export default "hello".endsWith("lo")`, Expected: true},
			{Name: "indexOf", Code: `export default "hello".indexOf("l")`, Expected: int64(2)},
			{Name: "replace", Code: `export default "hello".replace("l", "L")`, Expected: "heLlo"},
			{Name: "replaceAll", Code: `export default "hello".replaceAll("l", "L")`, Expected: "heLLo"},
			{Name: "padStart", Code: `export default "5".padStart(3, "0")`, Expected: "005"},
			{Name: "repeat", Code: `export default "ab".repeat(3)`, Expected: "ababab"},
			{Name: "charAt", Code: `export default "hello".charAt(1)`, Expected: "e"},
			{Name: "charCodeAt", Code: `export default "A".charCodeAt(0)`, Expected: int64(65)},
		},
	},
	{
		Name: "Array Operations",
		Tests: []TestCase{
			{Name: "array literal", Code: `export default [1, 2, 3].length`, Expected: int64(3)},
			{Name: "array index", Code: `const arr = [10, 20, 30]; export default arr[1]`, Expected: int64(20)},
			{Name: "push", Code: `const arr = [1, 2]; arr.push(3); export default arr.length`, Expected: int64(3)},
			{Name: "pop", Code: `const arr = [1, 2, 3]; export default arr.pop()`, Expected: int64(3)},
			{Name: "shift", Code: `const arr = [1, 2, 3]; export default arr.shift()`, Expected: int64(1)},
			{Name: "map", Code: `export default [1, 2, 3].map(x => x * 2).join(",")`, Expected: "2,4,6"},
			{Name: "filter", Code: `export default [1, 2, 3, 4].filter(x => x % 2 === 0).length`, Expected: int64(2)},
			{Name: "reduce", Code: `export default [1, 2, 3, 4].reduce((a, b) => a + b, 0)`, Expected: int64(10)},
			{Name: "find", Code: `export default [1, 2, 3].find(x => x > 1)`, Expected: int64(2)},
			{Name: "findIndex", Code: `export default [1, 2, 3].findIndex(x => x > 1)`, Expected: int64(1)},
			{Name: "some", Code: `export default [1, 2, 3].some(x => x > 2)`, Expected: true},
			{Name: "every", Code: `export default [1, 2, 3].every(x => x > 0)`, Expected: true},
			{Name: "includes", Code: `export default [1, 2, 3].includes(2)`, Expected: true},
			{Name: "indexOf", Code: `export default [1, 2, 3].indexOf(2)`, Expected: int64(1)},
			{Name: "join", Code: `export default [1, 2, 3].join("-")`, Expected: "1-2-3"},
			{Name: "reverse", Code: `export default [1, 2, 3].reverse().join(",")`, Expected: "3,2,1"},
			{Name: "slice", Code: `export default [1, 2, 3, 4].slice(1, 3).join(",")`, Expected: "2,3"},
			{Name: "concat", Code: `export default [1, 2].concat([3, 4]).length`, Expected: int64(4)},
			{Name: "flat", Code: `export default [[1], [2, 3]].flat().length`, Expected: int64(3)},
			{Name: "flatMap", Code: `export default [1, 2].flatMap(x => [x, x * 2]).join(",")`, Expected: "1,2,2,4"},
			{Name: "sort numbers", Code: `export default [3, 1, 2].sort((a, b) => a - b).join(",")`, Expected: "1,2,3"},
			{Name: "Array.from", Code: `export default Array.from("abc").join(",")`, Expected: "a,b,c"},
			{Name: "Array.isArray", Code: `export default Array.isArray([1, 2])`, Expected: true},
			{Name: "spread array", Code: `const a = [1, 2]; export default [...a, 3].length`, Expected: int64(3)},
		},
	},
	{
		Name: "Object Operations",
		Tests: []TestCase{
			{Name: "object literal", Code: `const obj = { a: 1, b: 2 }; export default obj.a + obj.b`, Expected: int64(3)},
			{Name: "computed property", Code: `const key = "name"; const obj = { [key]: "test" }; export default obj.name`, Expected: "test"},
			{Name: "shorthand property", Code: `const x = 1; const obj = { x }; export default obj.x`, Expected: int64(1)},
			{Name: "spread object", Code: `const a = { x: 1 }; const b = { ...a, y: 2 }; export default b.x + b.y`, Expected: int64(3)},
			{Name: "Object.keys", Code: `export default Object.keys({ a: 1, b: 2 }).length`, Expected: int64(2)},
			{Name: "Object.values", Code: `export default Object.values({ a: 1, b: 2 }).reduce((a: number, b: number) => a + b, 0)`, Expected: int64(3)},
			{Name: "Object.entries", Code: `export default Object.entries({ a: 1 }).length`, Expected: int64(1)},
			{Name: "Object.assign", Code: `const obj = Object.assign({}, { a: 1 }, { b: 2 }); export default obj.a + obj.b`, Expected: int64(3)},
			{Name: "hasOwnProperty", Code: `const obj = { a: 1 }; export default obj.hasOwnProperty("a")`, Expected: true},
			{Name: "in operator", Code: `const obj = { a: 1 }; export default "a" in obj`, Expected: true},
			{Name: "delete property", Code: `const obj: any = { a: 1, b: 2 }; delete obj.a; export default "a" in obj`, Expected: false},
			{Name: "optional chaining", Code: `const obj = { a: { b: 1 } }; export default obj.a?.b`, Expected: int64(1)},
			{Name: "destructuring", Code: `const { a, b } = { a: 1, b: 2 }; export default a + b`, Expected: int64(3)},
			{Name: "nested destructuring", Code: `const { a: { b } } = { a: { b: 42 } }; export default b`, Expected: int64(42)},
			{Name: "rest in destructuring", Code: `const { a, ...rest } = { a: 1, b: 2, c: 3 }; export default Object.keys(rest).length`, Expected: int64(2)},
		},
	},
	{
		Name: "Functions",
		Tests: []TestCase{
			{Name: "function declaration", Code: `function add(a: number, b: number) { return a + b; } export default add(2, 3)`, Expected: int64(5)},
			{Name: "arrow function", Code: `const add = (a: number, b: number) => a + b; export default add(2, 3)`, Expected: int64(5)},
			{Name: "arrow with block", Code: `const add = (a: number, b: number) => { return a + b; }; export default add(2, 3)`, Expected: int64(5)},
			{Name: "default parameters", Code: `const greet = (name = "World") => "Hello " + name; export default greet()`, Expected: "Hello World"},
			{Name: "rest parameters", Code: `const sum = (...nums: number[]) => nums.reduce((a, b) => a + b, 0); export default sum(1, 2, 3)`, Expected: int64(6)},
			{Name: "spread in call", Code: `const add = (a: number, b: number, c: number) => a + b + c; export default add(...[1, 2, 3])`, Expected: int64(6)},
			{Name: "closure", Code: `const makeCounter = () => { let n = 0; return () => ++n; }; const c = makeCounter(); c(); export default c()`, Expected: int64(2)},
			{Name: "IIFE", Code: `export default ((x: number) => x * 2)(21)`, Expected: int64(42)},
			{Name: "higher order", Code: `const twice = (f: (x: number) => number) => (x: number) => f(f(x)); const addOne = (x: number) => x + 1; export default twice(addOne)(0)`, Expected: int64(2)},
			{Name: "recursion", Code: `const fact = (n: number): number => n <= 1 ? 1 : n * fact(n - 1); export default fact(5)`, Expected: int64(120)},
		},
	},
	{
		Name: "TypeScript Types",
		Tests: []TestCase{
			{Name: "type annotation", Code: `const x: number = 42; export default x`, Expected: int64(42)},
			{Name: "interface", Code: `interface Point { x: number; y: number; } const p: Point = { x: 1, y: 2 }; export default p.x + p.y`, Expected: int64(3)},
			{Name: "type alias", Code: `type ID = number | string; const id: ID = 42; export default id`, Expected: int64(42)},
			{Name: "generic function", Code: `function identity<T>(x: T): T { return x; } export default identity(42)`, Expected: int64(42)},
			{Name: "generic array", Code: `function first<T>(arr: T[]): T | undefined { return arr[0]; } export default first([1, 2, 3])`, Expected: int64(1)},
			{Name: "union type", Code: `const x: number | string = "hello"; export default x`, Expected: "hello"},
			{Name: "literal type", Code: `const x: "a" | "b" = "a"; export default x`, Expected: "a"},
			{Name: "tuple", Code: `const t: [number, string] = [1, "a"]; export default t[0] + t[1]`, Expected: "1a"},
			{Name: "enum", Code: `enum Color { Red, Green, Blue } export default Color.Green`, Expected: int64(1)},
			{Name: "string enum", Code: `enum Dir { Up = "UP", Down = "DOWN" } export default Dir.Up`, Expected: "UP"},
			{Name: "type assertion", Code: `const x: any = "hello"; export default (x as string).length`, Expected: int64(5)},
			{Name: "keyof", Code: `interface Obj { a: number; b: string; } type Keys = keyof Obj; const k: Keys = "a"; export default k`, Expected: "a"},
			{Name: "readonly", Code: `interface Config { readonly host: string; } const c: Config = { host: "localhost" }; export default c.host`, Expected: "localhost"},
			{Name: "optional property", Code: `interface Opts { name?: string; } const o: Opts = {}; export default o.name ?? "default"`, Expected: "default"},
			{Name: "intersection type", Code: `type A = { a: number }; type B = { b: string }; type C = A & B; const c: C = { a: 1, b: "x" }; export default c.a`, Expected: int64(1)},
		},
	},
	{
		Name: "Classes",
		Tests: []TestCase{
			{Name: "class basic", Code: `class Point { x: number; y: number; constructor(x: number, y: number) { this.x = x; this.y = y; } } const p = new Point(1, 2); export default p.x + p.y`, Expected: int64(3)},
			{Name: "class method", Code: `class Counter { count = 0; inc() { this.count++; } } const c = new Counter(); c.inc(); c.inc(); export default c.count`, Expected: int64(2)},
			{Name: "getter setter", Code: `class Box { private _val = 0; get val() { return this._val; } set val(v) { this._val = v; } } const b = new Box(); b.val = 42; export default b.val`, Expected: int64(42)},
			{Name: "static method", Code: `class MathUtil { static double(x: number) { return x * 2; } } export default MathUtil.double(21)`, Expected: int64(42)},
			{Name: "static property", Code: `class Config { static version = "1.0"; } export default Config.version`, Expected: "1.0"},
			{Name: "inheritance", Code: `class Animal { speak() { return "..."; } } class Dog extends Animal { speak() { return "woof"; } } export default new Dog().speak()`, Expected: "woof"},
			{Name: "super call", Code: `class A { greet() { return "Hello"; } } class B extends A { greet() { return super.greet() + " World"; } } export default new B().greet()`, Expected: "Hello World"},
			{Name: "instanceof", Code: `class Foo {} const f = new Foo(); export default f instanceof Foo`, Expected: true},
		},
	},
	{
		Name: "Control Flow",
		Tests: []TestCase{
			{Name: "if true", Code: `let x = 0; if (true) { x = 1; } export default x`, Expected: int64(1)},
			{Name: "if false", Code: `let x = 0; if (false) { x = 1; } export default x`, Expected: int64(0)},
			{Name: "if else", Code: `const x = false ? 1 : 2; export default x`, Expected: int64(2)},
			{Name: "switch", Code: `const x = 2; let r = ""; switch (x) { case 1: r = "one"; break; case 2: r = "two"; break; default: r = "other"; } export default r`, Expected: "two"},
			{Name: "for loop", Code: `let sum = 0; for (let i = 1; i <= 5; i++) { sum += i; } export default sum`, Expected: int64(15)},
			{Name: "for of", Code: `let sum = 0; for (const x of [1, 2, 3]) { sum += x; } export default sum`, Expected: int64(6)},
			{Name: "for in", Code: `const obj = { a: 1, b: 2 }; let keys = ""; for (const k in obj) { keys += k; } export default keys.length`, Expected: int64(2)},
			{Name: "while", Code: `let i = 0; while (i < 5) { i++; } export default i`, Expected: int64(5)},
			{Name: "do while", Code: `let i = 0; do { i++; } while (i < 5); export default i`, Expected: int64(5)},
			{Name: "break", Code: `let i = 0; while (true) { i++; if (i === 5) break; } export default i`, Expected: int64(5)},
			{Name: "continue", Code: `let sum = 0; for (let i = 1; i <= 5; i++) { if (i === 3) continue; sum += i; } export default sum`, Expected: int64(12)},
		},
	},
	{
		Name: "Error Handling",
		Tests: []TestCase{
			{Name: "try catch", Code: `let r = ""; try { throw new Error("test"); } catch (e) { r = "caught"; } export default r`, Expected: "caught"},
			{Name: "try finally", Code: `let r = ""; try { r = "try"; } finally { r += "-finally"; } export default r`, Expected: "try-finally"},
			{Name: "error message", Code: `let msg = ""; try { throw new Error("oops"); } catch (e: any) { msg = e.message; } export default msg`, Expected: "oops"},
			{Name: "custom error", Code: `class MyError extends Error { code = 42; } let code = 0; try { throw new MyError(); } catch (e: any) { code = e.code; } export default code`, Expected: int64(42)},
		},
	},
	{
		Name: "Promises",
		Tests: []TestCase{
			{Name: "Promise.resolve", Code: `export default Promise.resolve(42).then(x => x)`, Compare: func(v any) bool { return fmt.Sprint(v) == "42" || v == int64(42) }},
			{Name: "Promise.reject catch", Code: `export default Promise.reject("err").catch(() => "caught")`, Compare: func(v any) bool { return v == "caught" }},
			{Name: "Promise.all", Code: `export default Promise.all([Promise.resolve(1), Promise.resolve(2)]).then((arr: number[]) => arr.reduce((a, b) => a + b, 0))`, Compare: func(v any) bool { return fmt.Sprint(v) == "3" || v == int64(3) }},
			{Name: "Promise chaining", Code: `export default Promise.resolve(1).then(x => x + 1).then(x => x + 1)`, Compare: func(v any) bool { return fmt.Sprint(v) == "3" || v == int64(3) }},
		},
	},
	{
		Name: "Math Operations",
		Tests: []TestCase{
			{Name: "Math.abs", Code: `export default Math.abs(-42)`, Expected: int64(42)},
			{Name: "Math.floor", Code: `export default Math.floor(3.7)`, Expected: int64(3)},
			{Name: "Math.ceil", Code: `export default Math.ceil(3.2)`, Expected: int64(4)},
			{Name: "Math.round", Code: `export default Math.round(3.5)`, Expected: int64(4)},
			{Name: "Math.max", Code: `export default Math.max(1, 5, 3)`, Expected: int64(5)},
			{Name: "Math.min", Code: `export default Math.min(1, 5, 3)`, Expected: int64(1)},
			{Name: "Math.pow", Code: `export default Math.pow(2, 8)`, Expected: int64(256)},
			{Name: "Math.sqrt", Code: `export default Math.sqrt(16)`, Expected: int64(4)},
			{Name: "Math.sign", Code: `export default Math.sign(-5)`, Expected: int64(-1)},
			{Name: "Math.trunc", Code: `export default Math.trunc(3.9)`, Expected: int64(3)},
			{Name: "Math.PI", Code: `export default Math.PI > 3.14 && Math.PI < 3.15`, Expected: true},
			{Name: "Math.random", Code: `const r = Math.random(); export default r >= 0 && r < 1`, Expected: true},
		},
	},
	{
		Name: "JSON Operations",
		Tests: []TestCase{
			{Name: "JSON.stringify", Code: `export default JSON.stringify({ a: 1 })`, Expected: `{"a":1}`},
			{Name: "JSON.parse", Code: `const obj = JSON.parse('{"a":1}'); export default obj.a`, Expected: int64(1)},
			{Name: "stringify array", Code: `export default JSON.stringify([1, 2, 3])`, Expected: "[1,2,3]"},
			{Name: "parse array", Code: `const arr = JSON.parse("[1,2,3]"); export default arr.length`, Expected: int64(3)},
			{Name: "nested object", Code: `const obj = JSON.parse('{"a":{"b":2}}'); export default obj.a.b`, Expected: int64(2)},
		},
	},
	{
		Name: "Date Operations",
		Tests: []TestCase{
			{Name: "Date.now", Code: `export default Date.now() > 0`, Expected: true},
			{Name: "new Date year", Code: `const d = new Date(2024, 0, 15); export default d.getFullYear()`, Expected: int64(2024)},
			{Name: "new Date month", Code: `const d = new Date(2024, 5, 15); export default d.getMonth()`, Expected: int64(5)},
			{Name: "Date getTime", Code: `const d = new Date(0); export default d.getTime()`, Expected: int64(0)},
			{Name: "Date toISOString", Code: `const d = new Date(0); export default d.toISOString().startsWith("1970")`, Expected: true},
		},
	},
	{
		Name: "RegExp Operations",
		Tests: []TestCase{
			{Name: "test match", Code: `export default /hello/.test("hello world")`, Expected: true},
			{Name: "test no match", Code: `export default /xyz/.test("hello world")`, Expected: false},
			{Name: "exec", Code: `const m = /(\w+)/.exec("hello"); export default m ? m[1] : ""`, Expected: "hello"},
			{Name: "match", Code: `const m = "hello world".match(/(\w+)/); export default m ? m[1] : ""`, Expected: "hello"},
			{Name: "replace regex", Code: `export default "hello world".replace(/o/g, "0")`, Expected: "hell0 w0rld"},
			// {Name: "split regex", Code: `export default "a1b2c".split(/\\d/).join("-")`, Expected: "a-b-c"},
			{Name: "case insensitive", Code: `export default /hello/i.test("HELLO")`, Expected: true},
			{Name: "global flag", Code: `export default "aaa".match(/a/g)?.length ?? 0`, Expected: int64(3)},
		},
	},
	{
		Name: "Set & Map",
		Tests: []TestCase{
			{Name: "Set add", Code: `const s = new Set(); s.add(1); s.add(2); s.add(1); export default s.size`, Expected: int64(2)},
			{Name: "Set has", Code: `const s = new Set([1, 2, 3]); export default s.has(2)`, Expected: true},
			{Name: "Set delete", Code: `const s = new Set([1, 2]); s.delete(1); export default s.size`, Expected: int64(1)},
			{Name: "Set from array", Code: `export default [...new Set([1, 1, 2, 2])].length`, Expected: int64(2)},
			{Name: "Map set get", Code: `const m = new Map(); m.set("a", 1); export default m.get("a")`, Expected: int64(1)},
			{Name: "Map has", Code: `const m = new Map([["a", 1]]); export default m.has("a")`, Expected: true},
			{Name: "Map size", Code: `const m = new Map([["a", 1], ["b", 2]]); export default m.size`, Expected: int64(2)},
			{Name: "Map delete", Code: `const m = new Map([["a", 1]]); m.delete("a"); export default m.size`, Expected: int64(0)},
			{Name: "Map keys", Code: `const m = new Map([["a", 1], ["b", 2]]); export default [...m.keys()].join(",")`, Expected: "a,b"},
		},
	},
	{
		Name: "Symbols",
		Tests: []TestCase{
			{Name: "Symbol create", Code: `const s = Symbol("test"); export default typeof s`, Expected: "symbol"},
			{Name: "Symbol.for", Code: `const s1 = Symbol.for("key"); const s2 = Symbol.for("key"); export default s1 === s2`, Expected: true},
		},
	},
	{
		Name: "Destructuring Advanced",
		Tests: []TestCase{
			{Name: "array destructure", Code: `const [a, b] = [1, 2]; export default a + b`, Expected: int64(3)},
			{Name: "array with default", Code: `const [a, b = 10] = [1]; export default b`, Expected: int64(10)},
			{Name: "array skip", Code: `const [, , third] = [1, 2, 3]; export default third`, Expected: int64(3)},
			{Name: "array rest", Code: `const [first, ...rest] = [1, 2, 3]; export default rest.length`, Expected: int64(2)},
			{Name: "object rename", Code: `const { a: x } = { a: 42 }; export default x`, Expected: int64(42)},
			{Name: "object default", Code: `const { a = 10 } = {} as { a?: number }; export default a`, Expected: int64(10)},
			{Name: "function param destruct", Code: `const fn = ({ x, y }: { x: number; y: number }) => x + y; export default fn({ x: 1, y: 2 })`, Expected: int64(3)},
			{Name: "swap variables", Code: `let a = 1, b = 2; [a, b] = [b, a]; export default a * 10 + b`, Expected: int64(21)},
		},
	},
	{
		Name: "Template Literals Advanced",
		Tests: []TestCase{
			{Name: "multiline", Code: "export default `line1\\nline2`.includes('\\n')", Expected: true},
			{Name: "expression", Code: "const x = 10; export default `value: ${x * 2}`", Expected: "value: 20"},
			{Name: "nested template", Code: "const inner = `world`; export default `hello ${inner}`", Expected: "hello world"},
		},
	},
	{
		Name: "Spread & Rest Advanced",
		Tests: []TestCase{
			{Name: "spread in array literal", Code: `export default [1, ...[2, 3], 4].length`, Expected: int64(4)},
			{Name: "spread in object literal", Code: `const o = { ...{ a: 1 }, ...{ b: 2 } }; export default o.a + o.b`, Expected: int64(3)},
			{Name: "spread string", Code: `export default [..."abc"].join("-")`, Expected: "a-b-c"},
			{Name: "rest in function", Code: `const fn = (a: number, ...rest: number[]) => rest.length; export default fn(1, 2, 3, 4)`, Expected: int64(3)},
			{Name: "spread Set", Code: `export default [...new Set([1, 2, 2])].length`, Expected: int64(2)},
		},
	},
	{
		Name: "Nullish & Optional",
		Tests: []TestCase{
			{Name: "nullish on null", Code: `const x = null; export default x ?? "default"`, Expected: "default"},
			{Name: "nullish on undefined", Code: `const x = undefined; export default x ?? "default"`, Expected: "default"},
			{Name: "nullish on zero", Code: `const x = 0; export default x ?? 10`, Expected: int64(0)},
			{Name: "nullish on empty string", Code: `const x = ""; export default x ?? "default"`, Expected: ""},
			{Name: "optional chain null", Code: `const o: any = null; export default o?.a ?? "none"`, Expected: "none"},
			{Name: "optional chain nested", Code: `const o = { a: { b: { c: 42 } } }; export default o?.a?.b?.c`, Expected: int64(42)},
			{Name: "optional chain method", Code: `const o: any = { fn: () => 42 }; export default o?.fn?.()`, Expected: int64(42)},
			{Name: "optional chain array", Code: `const arr = [1, 2, 3]; export default arr?.[1]`, Expected: int64(2)},
			{Name: "nullish assignment", Code: `let x: number | null = null; x ??= 42; export default x`, Expected: int64(42)},
			{Name: "logical or assign", Code: `let x = 0; x ||= 42; export default x`, Expected: int64(42)},
			{Name: "logical and assign", Code: `let x: number | null = 1; x &&= 42; export default x`, Expected: int64(42)},
		},
	},
	{
		Name: "Number Methods",
		Tests: []TestCase{
			{Name: "toFixed", Code: `export default (3.14159).toFixed(2)`, Expected: "3.14"},
			{Name: "toString radix", Code: `export default (255).toString(16)`, Expected: "ff"},
			{Name: "parseInt", Code: `export default parseInt("42")`, Expected: int64(42)},
			{Name: "parseFloat", Code: `export default parseFloat("3.14")`, Expected: 3.14},
			{Name: "Number.isInteger", Code: `export default Number.isInteger(42)`, Expected: true},
			{Name: "Number.isNaN", Code: `export default Number.isNaN(NaN)`, Expected: true},
			{Name: "Number.isFinite", Code: `export default Number.isFinite(42)`, Expected: true},
		},
	},
	{
		Name: "Object Static Methods",
		Tests: []TestCase{
			{Name: "Object.freeze", Code: `const o = Object.freeze({ a: 1 }); export default Object.isFrozen(o)`, Expected: true},
			{Name: "Object.seal", Code: `const o = Object.seal({ a: 1 }); export default Object.isSealed(o)`, Expected: true},
			{Name: "Object.getOwnPropertyNames", Code: `export default Object.getOwnPropertyNames({ a: 1, b: 2 }).length`, Expected: int64(2)},
		},
	},
	{
		Name: "Bitwise Operations",
		Tests: []TestCase{
			{Name: "AND", Code: `export default 5 & 3`, Expected: int64(1)},
			{Name: "OR", Code: `export default 5 | 3`, Expected: int64(7)},
			{Name: "XOR", Code: `export default 5 ^ 3`, Expected: int64(6)},
			{Name: "NOT", Code: `export default ~5`, Expected: int64(-6)},
			{Name: "left shift", Code: `export default 1 << 4`, Expected: int64(16)},
			{Name: "right shift", Code: `export default 16 >> 2`, Expected: int64(4)},
			{Name: "unsigned right shift", Code: `export default -1 >>> 0`, Expected: int64(4294967295)},
		},
	},
}

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    tsgo TypeScript Feature Test Suite                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	engines := []struct {
		name   string
		engine tsgo.EngineType
	}{
		{"GOJA", tsgo.EngineGOJA},
		{"Bun", tsgo.EngineBun},
	}

	ctx := context.Background()
	results := make(map[string]map[string]map[string]bool)

	for _, eng := range engines {
		results[eng.name] = make(map[string]map[string]bool)

		executor := tsgo.New(
			tsgo.WithEngine(eng.engine),
			tsgo.WithTimeout(10*time.Second),
		)

		for _, category := range testCategories {
			results[eng.name][category.Name] = make(map[string]bool)

			for _, test := range category.Tests {
				if test.Skip != "" {
					results[eng.name][category.Name][test.Name] = true
					continue
				}

				result, err := executor.Execute(ctx, test.Code)
				passed := false

				if err == nil {
					if test.Compare != nil {
						passed = test.Compare(result.Value)
					} else {
						passed = compareValues(result.Value, test.Expected)
					}
				}

				results[eng.name][category.Name][test.Name] = passed
			}
		}

		_ = executor.Close()
	}

	// Print results
	totalTests := 0
	gojaPass, bunPass := 0, 0
	bothPass, bothFail := 0, 0

	for _, category := range testCategories {
		fmt.Printf("┌─ %s ", category.Name)
		fmt.Println(strings.Repeat("─", 75-len(category.Name)) + "┐")

		for _, test := range category.Tests {
			totalTests++
			gojaPassed := results["GOJA"][category.Name][test.Name]
			bunPassed := results["Bun"][category.Name][test.Name]

			if gojaPassed {
				gojaPass++
			}
			if bunPassed {
				bunPass++
			}
			if gojaPassed && bunPassed {
				bothPass++
			}
			if !gojaPassed && !bunPassed {
				bothFail++
			}

			gojaIcon := "✓"
			if !gojaPassed {
				gojaIcon = "✗"
			}
			bunIcon := "✓"
			if !bunPassed {
				bunIcon = "✗"
			}

			status := "  "
			if !gojaPassed && !bunPassed {
				status = "!!"
			} else if gojaPassed != bunPassed {
				status = "!="
			}

			// Build content, then pad to fixed width (78 display chars inside borders)
			// Unicode icons are 1 display char but 3 bytes, so we calculate display width
			content := fmt.Sprintf(" %s %-40s  GOJA: %s  Bun: %s", status, test.Name, gojaIcon, bunIcon)
			// content display width: 1 + 2 + 1 + 40 + 8 + 1 + 7 + 1 = 61 chars
			// Need 78 - 61 = 17 spaces padding
			displayWidth := 1 + 2 + 1 + 40 + 8 + 1 + 7 + 1 // = 61
			padding := 78 - displayWidth
			fmt.Printf("│%s%s│\n", content, strings.Repeat(" ", padding))
		}

		fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")
		fmt.Println()
	}

	// Summary - using helper for proper alignment
	printBox := func(content string) {
		// Pad content to exactly 78 display chars
		padding := 78 - len(content)
		if padding < 0 {
			padding = 0
		}
		fmt.Printf("║%s%s║\n", content, strings.Repeat(" ", padding))
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	printBox("                                  Summary                                     ")
	fmt.Println("╠══════════════════════════════════════════════════════════════════════════════╣")
	printBox(fmt.Sprintf("  Total Tests:     %-d", totalTests))
	printBox(fmt.Sprintf("  GOJA Passed:     %d / %d (%5.1f%%)", gojaPass, totalTests, float64(gojaPass)/float64(totalTests)*100))
	printBox(fmt.Sprintf("  Bun Passed:      %d / %d (%5.1f%%)", bunPass, totalTests, float64(bunPass)/float64(totalTests)*100))
	printBox(fmt.Sprintf("  Both Passed:     %d / %d (%5.1f%%)", bothPass, totalTests, float64(bothPass)/float64(totalTests)*100))
	printBox(fmt.Sprintf("  Both Failed:     %d", bothFail))
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")

	if bothPass != totalTests {
		os.Exit(1)
	}
}

func compareValues(actual, expected any) bool {
	if actual == nil && expected == nil {
		return true
	}
	if actual == nil || expected == nil {
		return false
	}

	actualVal := reflect.ValueOf(actual)
	expectedVal := reflect.ValueOf(expected)

	if isNumeric(actualVal.Kind()) && isNumeric(expectedVal.Kind()) {
		actualFloat := toFloat64(actual)
		expectedFloat := toFloat64(expected)
		diff := actualFloat - expectedFloat
		if diff < 0 {
			diff = -diff
		}
		return diff < 0.0001
	}

	if actualVal.Kind() == reflect.String && expectedVal.Kind() == reflect.String {
		return actual.(string) == expected.(string)
	}

	if actualVal.Kind() == reflect.Bool && expectedVal.Kind() == reflect.Bool {
		return actual.(bool) == expected.(bool)
	}

	return fmt.Sprint(actual) == fmt.Sprint(expected)
}

func isNumeric(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int8:
		return float64(n)
	case int16:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint8:
		return float64(n)
	case uint16:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}
