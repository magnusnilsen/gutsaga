-- SavedVariables rename: empty SagaloreDB takes OurSagasDB; progress is kept.
local dir = arg[0]:match("^(.*)/") or "."
local M = dofile(dir .. "/Sagalore/SagaloreMigrate.lua")

local function assertTrue(cond, msg)
    if not cond then
        error(msg or "assertion failed", 2)
    end
end

local legacy = {
    completions = { ["abc"] = { character = "Magni" } },
    objectives = {},
}
assertTrue(M.apply(nil, legacy) == legacy, "nil current takes legacy")
assertTrue(M.apply({}, legacy) == legacy, "empty current takes legacy")

local fresh = { completions = {}, objectives = {}, read = {} }
assertTrue(M.apply(fresh, legacy) == legacy, "tables without progress take legacy")

local current = { completions = { ["new"] = true } }
assertTrue(M.apply(current, legacy) == current, "current progress wins")
assertTrue(M.apply(current, nil) == current, "missing legacy keeps current")
assertTrue(M.apply(nil, nil) == nil, "both missing")

local noticed = { seen_notice = "n1", completions = {} }
assertTrue(M.hasProgress(noticed), "seen notice is progress")
assertTrue(M.apply(noticed, legacy) == noticed, "notice keeps the new table")

print("migrate_test ok")
