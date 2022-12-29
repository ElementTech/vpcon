package main

import (
	"flag"
	"fmt"
	"net"

	cidrman "github.com/EvilSuperstars/go-cidrman"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Define command line flags for the VPC IDs
	vpc1ID := flag.String("vpc1", "", "The ID of the first VPC")
	vpc2ID := flag.String("vpc2", "", "The ID of the second VPC")
	flag.Parse()

	// Make sure the VPC IDs are provided
	if *vpc1ID == "" || *vpc2ID == "" {
		log.Error("Error: VPC IDs must be provided")
		return
	}

	// Create an EC2 client
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc := ec2.New(sess)

	CheckTableHasRouteToVPC(svc, vpc1ID, vpc2ID, "VPC 1", "VPC 2")
	CheckTableHasRouteToVPC(svc, vpc2ID, vpc1ID, "VPC 2", "VPC 1")
}

func CheckTableHasRouteToVPC(svc *ec2.EC2, vpc1ID, vpc2ID *string, sourceName, targetName string) bool {
	rtOutput, err := svc.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{vpc1ID},
			},
		},
	})
	if err != nil {
		log.Error("Error getting route table for VPC:", err)
		return false
	}
	// Get the subnets in the VPC
	subnetsOutput, err := svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{vpc2ID},
			},
		},
	})

	if err != nil {
		log.Error("Error getting subnets:", err)
		return false
	}
	vpc2CidrBlock := []string{}
	// Print the CIDR blocks of the subnets
	for _, s := range subnetsOutput.Subnets {
		if s.CidrBlock != nil {
			vpc2CidrBlock = append(vpc2CidrBlock, *s.CidrBlock)
		}
	}
	mergedCidr, err := cidrman.MergeCIDRs(vpc2CidrBlock)
	if err != nil {
		log.Error("Error merging subnets:", err)
		return false
	}
	for _, rt := range rtOutput.RouteTables {
		for _, r := range rt.Routes {
			if r.DestinationCidrBlock != nil {
				for _, vpc2SubnetCidr := range mergedCidr {
					_, ipnetA, _ := net.ParseCIDR(*r.DestinationCidrBlock)
					ipB, _, _ := net.ParseCIDR(vpc2SubnetCidr)
					if ipnetA.Contains(ipB) {
						if r.VpcPeeringConnectionId != nil {
							log.WithFields(log.Fields{"ID": *vpc1ID, "Route Table": *rt.RouteTableId, "Destination": *r.DestinationCidrBlock}).Info(fmt.Sprintf("%v -> Peering -> %v", sourceName, targetName))
							return true
						}
						if r.TransitGatewayId != nil {
							log.WithFields(log.Fields{"ID": *vpc1ID, "Route Table": *rt.RouteTableId, "Destination": *r.DestinationCidrBlock}).Info(fmt.Sprintf("%v -> TGW -> %v", sourceName, targetName))
							return true
						}
					}
				}

			}
		}
	}
	return false
}
