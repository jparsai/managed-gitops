package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllClusterUsers(ctx context.Context, clusterUsers *[]ClusterUser) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(clusterUsers).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteClusterUserById(ctx context.Context, id string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	result := &ClusterUser{}

	deleteResult, err := dbq.dbConnection.Model(result).
		Where("clusteruser_id = ?", id).
		Context(ctx).
		Delete()

	if err != nil {
		return 0, fmt.Errorf("error on deleting cluster_user: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateClusterUser(ctx context.Context, obj *ClusterUser) error {

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if dbq.allowTestUuids {
		if IsEmpty(obj.Clusteruser_id) {
			obj.Clusteruser_id = generateUuid()
		}
	} else {
		if !IsEmpty(obj.Clusteruser_id) {
			return fmt.Errorf("primary key should be empty")
		}

		obj.Clusteruser_id = generateUuid()
	}

	// State

	if IsEmpty(obj.User_name) {
		return fmt.Errorf("user name should not be empty")
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting cluster user: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetClusterUserByUsername(ctx context.Context, clusterUser *ClusterUser) error {

	// TODO: GITOPSRVCE-68 - PERF - Add an index for this, if anything actually calls it

	if err := validateQueryParamsEntity(clusterUser, dbq); err != nil {
		return err
	}

	if IsEmpty(clusterUser.User_name) {
		return fmt.Errorf("username is nil for GetClusterUserByUsername")
	}

	var dbResults []ClusterUser

	if err := dbq.dbConnection.Model(&dbResults).
		Where("cu.user_name = ?", clusterUser.User_name).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving GetClusterUserByUsername: %v", err)
	}

	if len(dbResults) >= 2 {
		return fmt.Errorf("multiple results returned from GetClusterUserByUsername")
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("no results found for GetClusterUserByUsername")
	}

	*clusterUser = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetClusterUserById(ctx context.Context, clusterUser *ClusterUser) error {

	if err := validateQueryParamsEntity(clusterUser, dbq); err != nil {
		return err
	}

	if IsEmpty(clusterUser.Clusteruser_id) {
		return fmt.Errorf("cluster user id is empty")
	}

	var dbResults []ClusterUser

	if err := dbq.dbConnection.Model(&dbResults).
		Where("cu.clusteruser_id = ?", clusterUser.Clusteruser_id).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving GetClusterUserById: %v", err)
	}

	if len(dbResults) >= 2 {
		return fmt.Errorf("multiple results returned from GetClusterUserById")
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("no results found for GetClusterUserById")
	}

	*clusterUser = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetOrCreateSpecialClusterUser(ctx context.Context, clusterUser *ClusterUser) error {
	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Set the Special Cluster User details.
	specialClusterUserName := "cluster-agent-application-sync-user"
	var dbResults []ClusterUser

	// Check if SpecialClusterUser already exists.
	if err := dbq.dbConnection.Model(&dbResults).
		Where("cu.clusteruser_id = ?", specialClusterUserName).
		Context(ctx).
		Select(); err != nil {
		return fmt.Errorf("error on retrieving SpecialClusterUser: %v", err)
	}

	// If user already exists then return it, else create new.
	if len(dbResults) != 0 {
		*clusterUser = dbResults[0]
	} else {
		clusterUser.Clusteruser_id = specialClusterUserName
		clusterUser.User_name = "cluster-agent-application-sync-user"

		if _, err := dbq.dbConnection.Model(clusterUser).Context(ctx).Insert(); err != nil {
			return fmt.Errorf("error on inserting SpecialClusterUser: %v", err)
		}
	}
	return nil
}
