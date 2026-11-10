package store

import (
	"path/filepath"
	"testing"
)

func TestMigrationRepairBatch(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	// Повторно накатываем именно 012 на состояние предыдущей версии.
	exec(`DELETE FROM schema_migrations WHERE name = '012_repair_movement_batch.sql'`)
	exec(`INSERT INTO users (id,login,password_hash) VALUES (1,'a','x'),(2,'b','x')`)
	exec(`INSERT INTO categories (id,user_id,name,color) VALUES (1,1,'К','blue'),(2,2,'К','blue')`)
	exec(`INSERT INTO products (id,user_id,category_id,name,unit,package_size) VALUES (1,1,1,'П','ml',1000),(2,2,2,'П','ml',1000)`)
	exec(`INSERT INTO units (id,user_id,product_id,initial_qty,remaining_qty) VALUES (1,1,1,1000,500),(2,2,2,1000,500)`)
	exec(`INSERT INTO movements (id,user_id,unit_id,type,qty_delta,batch_id,created_at) VALUES
 (10,1,1,'consume',-1,8,100),(11,1,1,'consume',-1,8,100),
 (12,2,2,'consume',-1,8,100),(13,2,2,'consume',-1,8,100),
 (20,1,1,'consume',-1,20,100),(21,1,1,'consume',-1,20,100),
 (30,1,1,'consume',-1,NULL,100),(31,1,1,'consume',-1,NULL,100),
 (40,1,1,'consume',-1,42,100),(41,1,1,'consume',-1,42,100),
 (42,2,2,'consume',-1,42,200),
 (50,1,1,'consume',-1,52,100),(51,1,1,'consume',-1,52,100),
 (52,1,1,'consume',-1,53,200),(53,1,1,'consume',-1,53,200)`)
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	want := map[int]int{10: 10, 11: 10, 12: 12, 13: 12, 20: 20, 21: 20, 30: 30, 31: 31, 40: 40, 41: 40, 42: 42, 50: 50, 51: 50, 52: 53, 53: 53}
	for id, batch := range want {
		var got int
		if err := db.QueryRow(`SELECT batch_id FROM movements WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != batch {
			t.Errorf("движение %d: группа %d, нужна %d", id, got, batch)
		}
	}
	applied, err := db.Migrate()
	if err != nil || len(applied) != 0 {
		t.Fatalf("повторная миграция: %v, %v", applied, err)
	}
}
