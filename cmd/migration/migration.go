package main

import (
	"github.com/AlexBurnes/version-go/pkg/version"
	"github.com/guverz/migration-go/pkg/migration"
)

func Add_Migration() {
	migration.Add()
}

func Check_Migration() {
	migration.Check()
}

func Collect_Migration() {
	migration.Collect()
}

func Get_Ver() {
	version.GetVersion()
}
