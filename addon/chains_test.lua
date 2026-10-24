local src = debug.getinfo(1, "S").source
local dir = src:match("^@(.*)[/\\][^/\\]+$") or "."
local M = dofile(dir .. "/GutSaga/GutSagaChains.lua")

local function assertTrue(cond, msg)
    if not cond then
        error(msg or "assertion failed", 2)
    end
end

local entries = {
    { quest_id = "a", title = "En kveld", chain_id = "goldshire", chain_name = "Goldshire", chain_order = 1, chain_index = 1, chain_total = 4 },
    {
        quest_id = "b", title = "Veien", chain_id = "goldshire", chain_name = "Goldshire", chain_order = 2, chain_index = 2, chain_total = 4,
        requires = { { id = "a", title = "En kveld" } },
    },
    {
        quest_id = "c", title = "Melk", chain_id = "goldshire", chain_name = "Goldshire", chain_order = 3, chain_index = 3, chain_total = 4,
        requires = { "a", "b" },
    },
    { quest_id = "d", title = "Siste", chain_id = "goldshire", chain_name = "Goldshire", chain_order = 4 },
}

assertTrue(M.chapterLabel(entries[2]) == "Kapittel 2 av 4", "server index")
assertTrue(M.chapterLabel({ quest_id = "b", chain_id = "goldshire", chain_order = 2 }, entries) == "Kapittel 2 av 4", "local rank")
assertTrue(M.lockText(M.missing(entries[2], entries, {})) == "Låst: fullfør En kveld først", "one prereq")
assertTrue(M.isLocked(entries[2], entries, {}), "locked before")
assertTrue(not M.isLocked(entries[2], entries, { a = true }), "any completion unlocks, version ignored")
assertTrue(M.lockText(M.missing(entries[3], entries, {})) == "Låst: fullfør En kveld og Veien først", "two titles")
assertTrue(M.listSub(entries[2], entries, {}, "Ukjent") == "Kapittel 2 av 4 — Låst: fullfør En kveld først", "list lock")
assertTrue(M.listSub(entries[2], entries, { a = true }, "Ukjent") == "Goldshire · Kapittel 2 av 4", "list chapter")

local unlocked = M.newlyUnlocked(entries, {}, { a = true })
assertTrue(#unlocked == 1 and unlocked[1].quest_id == "b", "local unlock of the next chapter")
assertTrue(not M.isLocked(entries[1], entries, { a = true }), "finished saga is not locked")

local detail = M.detailLines(entries[2], entries, {})
assertTrue(detail:find("Kapittel 2 av 4", 1, true) ~= nil, "detail chapter")
assertTrue(detail:find("Låst: fullfør En kveld først", 1, true) ~= nil, "detail lock")

print("chains ok")
